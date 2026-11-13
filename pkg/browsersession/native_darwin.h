#include <stdint.h>
char *browser_start(void);
char *browser_create(const char *identifier, const char *profile, const char *origin, const char *initialURL);
void browser_eval(uint64_t token, const char *identifier, const char *source);
char *browser_show(const char *identifier, double x, double y, double width, double height, double viewportWidth, double viewportHeight);
void browser_hide(void);
void browser_destroy(const char *identifier);
void browser_close(void);
extern void browser_result(uint64_t token, char *value, char *error);
extern int browser_can_execute(uint64_t token);
