#ifndef API_SHADOW_CAPTURE_H
#define API_SHADOW_CAPTURE_H

#ifdef __cplusplus
extern "C" {
#endif

const char *apishadow_capture_version(void);
char *apishadow_capture_normalize(const char *record_json);
char *apishadow_capture_fingerprint(const char *record_json);
void apishadow_capture_free(char *ptr);

#ifdef __cplusplus
}
#endif

#endif
