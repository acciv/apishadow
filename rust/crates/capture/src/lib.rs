//! apishadow-capture — нормализация зеркального API-трафика.
//! Без внешних зависимостей. Работает с JSON-записями, ничего не расшифровывает.

use std::collections::BTreeMap;
use std::ffi::{c_char, CStr, CString};

pub const VERSION: &str = "0.1.0";

#[derive(Debug, Clone, PartialEq)]
enum Json {
    Str(String),
    Num(f64),
    Bool(bool),
    Obj(BTreeMap<String, Json>),
    Arr(Vec<Json>),
    Null,
}

fn skip_ws(bytes: &[u8], at: &mut usize) {
    while *at < bytes.len() && bytes[*at].is_ascii_whitespace() {
        *at += 1;
    }
}

fn parse_string(bytes: &[u8], at: &mut usize) -> Option<String> {
    if bytes.get(*at) != Some(&b'"') {
        return None;
    }
    *at += 1;
    let mut out = String::new();
    while *at < bytes.len() {
        let ch = bytes[*at];
        *at += 1;
        match ch {
            b'"' => return Some(out),
            b'\\' => {
                let next = *bytes.get(*at)?;
                *at += 1;
                match next {
                    b'n' => out.push('\n'),
                    b'r' => out.push('\r'),
                    b't' => out.push('\t'),
                    b'"' => out.push('"'),
                    b'\\' => out.push('\\'),
                    b'/' => out.push('/'),
                    b'b' => out.push('\u{8}'),
                    b'f' => out.push('\u{c}'),
                    b'u' => {
                        if *at + 4 > bytes.len() {
                            return None;
                        }
                        let hex = std::str::from_utf8(&bytes[*at..*at + 4]).ok()?;
                        let code = u32::from_str_radix(hex, 16).ok()?;
                        *at += 4;
                        out.push(char::from_u32(code).unwrap_or('\u{fffd}'));
                    }
                    _ => out.push(next as char),
                }
            }
            _ => out.push(ch as char),
        }
    }
    None
}

fn parse_value(bytes: &[u8], at: &mut usize) -> Option<Json> {
    skip_ws(bytes, at);
    match bytes.get(*at)? {
        b'"' => parse_string(bytes, at).map(Json::Str),
        b'{' => {
            *at += 1;
            let mut map = BTreeMap::new();
            loop {
                skip_ws(bytes, at);
                if bytes.get(*at) == Some(&b'}') {
                    *at += 1;
                    return Some(Json::Obj(map));
                }
                let key = parse_string(bytes, at)?;
                skip_ws(bytes, at);
                if bytes.get(*at) != Some(&b':') {
                    return None;
                }
                *at += 1;
                let value = parse_value(bytes, at)?;
                map.insert(key, value);
                skip_ws(bytes, at);
                match bytes.get(*at) {
                    Some(b',') => *at += 1,
                    Some(b'}') => {
                        *at += 1;
                        return Some(Json::Obj(map));
                    }
                    _ => return None,
                }
            }
        }
        b'[' => {
            *at += 1;
            let mut items = Vec::new();
            loop {
                skip_ws(bytes, at);
                if bytes.get(*at) == Some(&b']') {
                    *at += 1;
                    return Some(Json::Arr(items));
                }
                items.push(parse_value(bytes, at)?);
                skip_ws(bytes, at);
                match bytes.get(*at) {
                    Some(b',') => *at += 1,
                    Some(b']') => {
                        *at += 1;
                        return Some(Json::Arr(items));
                    }
                    _ => return None,
                }
            }
        }
        b't' if bytes[*at..].starts_with(b"true") => {
            *at += 4;
            Some(Json::Bool(true))
        }
        b'f' if bytes[*at..].starts_with(b"false") => {
            *at += 5;
            Some(Json::Bool(false))
        }
        b'n' if bytes[*at..].starts_with(b"null") => {
            *at += 4;
            Some(Json::Null)
        }
        b'-' | b'0'..=b'9' => {
            let start = *at;
            while *at < bytes.len()
                && (bytes[*at].is_ascii_digit()
                    || matches!(bytes[*at], b'-' | b'+' | b'.' | b'e' | b'E'))
            {
                *at += 1;
            }
            std::str::from_utf8(&bytes[start..*at])
                .ok()?
                .parse::<f64>()
                .ok()
                .map(Json::Num)
        }
        _ => None,
    }
}

fn parse_json(input: &str) -> Option<Json> {
    let mut at = 0usize;
    let value = parse_value(input.as_bytes(), &mut at)?;
    skip_ws(input.as_bytes(), &mut at);
    if at == input.len() {
        Some(value)
    } else {
        None
    }
}

pub fn json_escape(input: &str) -> String {
    let mut out = String::with_capacity(input.len() + 2);
    for ch in input.chars() {
        match ch {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if c.is_control() => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out
}

fn jstr(input: &str) -> String {
    format!("\"{}\"", json_escape(input))
}

fn to_json(value: &Json) -> String {
    match value {
        Json::Str(s) => jstr(s),
        Json::Num(n) if n.fract() == 0.0 && n.is_finite() => format!("{}", *n as i64),
        Json::Num(n) => format!("{}", n),
        Json::Bool(b) => b.to_string(),
        Json::Null => "null".into(),
        Json::Arr(items) => format!("[{}]", items.iter().map(to_json).collect::<Vec<_>>().join(",")),
        Json::Obj(map) => format!(
            "{{{}}}",
            map.iter()
                .map(|(k, v)| format!("{}:{}", jstr(k), to_json(v)))
                .collect::<Vec<_>>()
                .join(",")
        ),
    }
}

fn get<'a>(map: &'a BTreeMap<String, Json>, key: &str) -> Option<&'a Json> {
    map.get(key)
}

fn get_str(map: &BTreeMap<String, Json>, key: &str) -> String {
    match get(map, key) {
        Some(Json::Str(s)) => s.clone(),
        Some(Json::Num(n)) => format!("{}", n),
        _ => String::new(),
    }
}

fn get_f64(map: &BTreeMap<String, Json>, key: &str) -> f64 {
    match get(map, key) {
        Some(Json::Num(n)) => *n,
        _ => 0.0,
    }
}

fn fnv64(input: &str) -> u64 {
    let mut hash = 0xcbf2_9ce4_8422_2325u64;
    for byte in input.bytes() {
        hash ^= byte as u64;
        hash = hash.wrapping_mul(0x0100_0000_01b3);
    }
    hash
}

fn volatile(key: &str) -> bool {
    let k = key.to_ascii_lowercase();
    [
        "date", "server", "connection", "content-length", "set-cookie", "cookie",
        "x-request-id", "x-trace-id", "x-correlation-id", "cf-ray", "x-envoy",
        "x-amzn-requestid", "x-amz-request-id", "x-runtime", "age", "via",
    ]
    .iter()
    .any(|v| k == *v || k.starts_with(v))
}

/// Убирает нестабильные заголовки и приводит запись к каноническому виду.
pub fn normalize(input: &str) -> Result<String, String> {
    let Some(Json::Obj(source)) = parse_json(input) else {
        return Err("expected JSON object".into());
    };

    let mut headers = BTreeMap::new();
    if let Some(Json::Obj(raw)) = get(&source, "headers") {
        for (key, value) in raw {
            if volatile(key) {
                continue;
            }
            let text = match value {
                Json::Str(s) => s.clone(),
                Json::Num(n) => format!("{}", n),
                Json::Bool(b) => b.to_string(),
                _ => String::new(),
            };
            headers.insert(key.to_ascii_lowercase(), Json::Str(text));
        }
    }

    let body = get_str(&source, "body");
    let duration = get_f64(&source, "duration_ms");
    let ts = get_f64(&source, "ts");
    let mut out = BTreeMap::new();
    out.insert("body".into(), Json::Str(body.clone()));
    out.insert("body_fp".into(), Json::Str(format!("{:016x}", fnv64(&body))));
    out.insert("body_len".into(), Json::Num(body.len() as f64));
    out.insert("duration_ms".into(), Json::Num(duration));
    out.insert("headers".into(), Json::Obj(headers));
    out.insert("id".into(), Json::Str(get_str(&source, "id")));
    out.insert("method".into(), Json::Str(get_str(&source, "method")));
    out.insert("normalized".into(), Json::Bool(true));
    out.insert("path".into(), Json::Str(get_str(&source, "path")));
    out.insert("query".into(), Json::Str(get_str(&source, "query")));
    out.insert("role".into(), Json::Str(get_str(&source, "role")));
    out.insert("status".into(), Json::Num(get_f64(&source, "status")));
    out.insert("ts".into(), Json::Num(ts));
    out.insert("capture_version".into(), Json::Str(VERSION.into()));
    Ok(to_json(&Json::Obj(out)))
}

/// Отпечаток ответа без id, времени и длительности — для сравнения primary и shadow.
pub fn fingerprint(input: &str) -> Result<String, String> {
    let normalized = normalize(input)?;
    let Some(Json::Obj(map)) = parse_json(&normalized) else {
        return Err("normalize failed".into());
    };
    let mut stable = BTreeMap::new();
    for key in ["body", "headers", "method", "path", "query", "status"] {
        if let Some(value) = get(&map, key) {
            stable.insert(key.to_string(), value.clone());
        }
    }
    let text = to_json(&Json::Obj(stable));
    Ok(format!("{:016x}", fnv64(&text)))
}

fn read_input(ptr: *const c_char) -> Option<String> {
    if ptr.is_null() {
        return None;
    }
    unsafe { CStr::from_ptr(ptr) }.to_str().ok().map(str::to_string)
}

fn to_c(value: String) -> *mut c_char {
    CString::new(value)
        .unwrap_or_else(|_| CString::new("{\"error\":\"invalid string\"}").unwrap())
        .into_raw()
}

fn error(message: &str) -> String {
    format!("{{\"error\":\"{}\"}}", json_escape(message))
}

#[no_mangle]
pub extern "C" fn apishadow_capture_version() -> *const c_char {
    concat!(env!("CARGO_PKG_VERSION"), "\0").as_ptr() as *const c_char
}

#[no_mangle]
pub extern "C" fn apishadow_capture_normalize(input: *const c_char) -> *mut c_char {
    match read_input(input) {
        Some(value) => match normalize(&value) {
            Ok(result) => to_c(result),
            Err(err) => to_c(error(&err)),
        },
        None => to_c(error("null input")),
    }
}

#[no_mangle]
pub extern "C" fn apishadow_capture_fingerprint(input: *const c_char) -> *mut c_char {
    match read_input(input) {
        Some(value) => match fingerprint(&value) {
            Ok(result) => to_c(format!("{{\"fingerprint\":\"{}\"}}", result)),
            Err(err) => to_c(error(&err)),
        },
        None => to_c(error("null input")),
    }
}

#[no_mangle]
pub extern "C" fn apishadow_capture_free(ptr: *mut c_char) {
    if !ptr.is_null() {
        unsafe {
            drop(CString::from_raw(ptr));
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn record() -> &'static str {
        r#"{"id":"r1","role":"primary","ts":1,"method":"GET","path":"/v1/users","query":"","status":200,"duration_ms":12,"headers":{"Content-Type":"application/json","Date":"Mon, 1 Jan 2024 00:00:00 GMT","X-Request-Id":"abc"},"body":"{\"id\":1}"}"#
    }

    #[test]
    fn drops_volatile_headers() {
        let result = normalize(record()).unwrap();
        assert!(!result.contains("date"));
        assert!(!result.contains("x-request-id"));
        assert!(result.contains("content-type"));
        assert!(result.contains("body_fp"));
    }

    #[test]
    fn fingerprint_is_stable() {
        let a = fingerprint(record()).unwrap();
        let b = fingerprint(record()).unwrap();
        assert_eq!(a, b);
        assert_eq!(a.len(), 16);
    }

    #[test]
    fn fingerprint_ignores_timing_and_id() {
        let changed = record().replace("\"id\":\"r1\"", "\"id\":\"r2\"").replace("\"duration_ms\":12", "\"duration_ms\":99");
        assert_eq!(fingerprint(record()).unwrap(), fingerprint(&changed).unwrap());
    }

    #[test]
    fn invalid_json_is_error() {
        assert!(normalize("not json").is_err());
    }
}
