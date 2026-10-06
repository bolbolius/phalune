#ifndef COMPAT_PANGO_H
#define COMPAT_PANGO_H

#include_next <pango/pango.h>

#if !defined(PANGO_VERSION_CHECK) || !PANGO_VERSION_CHECK(1, 57, 0)
// Pango 1.57+ compatibility shims for gotk4 on systems with Pango <= 1.56 (e.g. Void Linux, Debian, Ubuntu LTS)
typedef enum {
    PANGO_FONT_COLOR_DONT_CARE = 0,
    PANGO_FONT_COLOR_REQUIRED = 1,
    PANGO_FONT_COLOR_FORBIDDEN = 2
} PangoFontColor;

static inline GType pango_font_color_get_type(void) {
    return G_TYPE_NONE;
}

static inline PangoFontColor pango_font_description_get_color(const PangoFontDescription *desc) {
    (void)desc;
    return PANGO_FONT_COLOR_DONT_CARE;
}

static inline void pango_font_description_set_color(PangoFontDescription *desc, PangoFontColor color) {
    (void)desc;
    (void)color;
}
#endif

#endif
