package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/acciv/apishadow/internal/capture"
)

type Stats struct {
	Requests     atomic.Int64
	Mirrored     atomic.Int64
	ShadowErrors atomic.Int64
	PrimaryErrors atomic.Int64
}

type Proxy struct {
	primary string
	shadow  string
	capture *capture.Writer
	client  *http.Client
	ratio   float64
	timeout time.Duration
	id      atomic.Uint64
	stats   *Stats
}

func New(primary, shadow string, writer *capture.Writer, ratio float64, timeout time.Duration) *Proxy {
	return &Proxy{
		primary: primary,
		shadow:  shadow,
		capture: writer,
		client:  &http.Client{Timeout: timeout},
		ratio:   ratio,
		timeout: timeout,
		stats:   &Stats{},
	}
}

func (p *Proxy) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"capture":   capture.Version(),
			"primary":   p.primary,
			"shadow":    p.shadow,
			"mirror_at": p.ratio,
		})
	})
	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"requests":      p.stats.Requests.Load(),
			"mirrored":      p.stats.Mirrored.Load(),
			"shadow_errors": p.stats.ShadowErrors.Load(),
			"primary_errors": p.stats.PrimaryErrors.Load(),
		})
	})
	mux.HandleFunc("/", p.ServeHTTP)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func flatHeaders(header http.Header) map[string]string {
	out := make(map[string]string, len(header))
	for key, values := range header {
		if len(values) > 0 {
			out[strings.ToLower(key)] = values[0]
		}
	}
	return out
}

func (p *Proxy) forward(ctx context.Context, r *http.Request, base string, id string, role string, body []byte) (*http.Response, time.Duration, error) {
	target, err := url.Parse(base)
	if err != nil {
		return nil, 0, err
	}
	u := *target
	u.Path = path.Join(target.Path, r.URL.Path)
	u.RawQuery = r.URL.RawQuery

	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header = r.Header.Clone()
	req.Header.Del("X-ApiShadow-Id")
	req.Header.Del("X-ApiShadow-Role")
	req.Header.Set("X-ApiShadow-Id", id)
	req.Header.Set("X-ApiShadow-Role", role)

	start := time.Now()
	resp, err := p.client.Do(req)
	return resp, time.Since(start), err
}

func readUpstream(resp *http.Response) (int, http.Header, []byte, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, resp.Header.Clone(), data, err
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.stats.Requests.Add(1)
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read body"})
		return
	}

	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), p.id.Add(1))
	resp, elapsed, err := p.forward(r.Context(), r, p.primary, id, "primary", body)
	if err != nil {
		p.stats.PrimaryErrors.Add(1)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "primary unavailable", "id": id})
		return
	}
	status, headers, payload, err := readUpstream(resp)
	if err != nil {
		p.stats.PrimaryErrors.Add(1)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "primary read failed", "id": id})
		return
	}

	for key, values := range headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("X-ApiShadow-Id", id)
	w.WriteHeader(status)
	_, _ = w.Write(payload)

	if p.capture != nil {
		_ = p.capture.Write(capture.Record{
			ID: id, Role: "primary", TS: float64(time.Now().UnixNano()) / 1e9,
			Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery,
			Status: status, Headers: flatHeaders(headers),
			Body: string(payload), DurationMS: float64(elapsed.Microseconds()) / 1000,
		})
	}

	if p.ratio <= 0 || rand.Float64() >= p.ratio {
		return
	}
	p.stats.Mirrored.Add(1)

	shadowBody := append([]byte(nil), body...)
	shadowReq := r.Clone(context.Background())
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
		defer cancel()
		shadowResp, dur, err := p.forward(ctx, shadowReq, p.shadow, id, "shadow", shadowBody)
		if err != nil {
			p.stats.ShadowErrors.Add(1)
			return
		}
		shadowStatus, shadowHeaders, shadowPayload, err := readUpstream(shadowResp)
		if err != nil {
			p.stats.ShadowErrors.Add(1)
			return
		}
		if p.capture != nil {
			_ = p.capture.Write(capture.Record{
				ID: id, Role: "shadow", TS: float64(time.Now().UnixNano()) / 1e9,
				Method: shadowReq.Method, Path: shadowReq.URL.Path, Query: shadowReq.URL.RawQuery,
				Status: shadowStatus, Headers: flatHeaders(shadowHeaders),
				Body: string(shadowPayload), DurationMS: float64(dur.Microseconds()) / 1000,
			})
		}
	}()
}

func ParseRatio(value string) float64 {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 1
	}
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}
