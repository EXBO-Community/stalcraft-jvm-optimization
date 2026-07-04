package i18n

import "strings"

var russianDefaultRegions = map[string]struct{}{
	"AM": {},
	"AZ": {},
	"BY": {},
	"GE": {},
	"KZ": {},
	"KG": {},
	"MD": {},
	"RU": {},
	"TJ": {},
	"TM": {},
	"UA": {},
	"UZ": {},
}

var russianDefaultLanguages = map[string]struct{}{
	"az": {},
	"be": {},
	"hy": {},
	"ka": {},
	"kk": {},
	"ky": {},
	"ru": {},
	"tg": {},
	"tk": {},
	"uk": {},
	"uz": {},
}

func Default() Language {
	locale := cleanLocale(systemLocale())
	language := localeLanguage(locale)
	region := localeRegion(locale)

	if _, ok := russianDefaultRegions[strings.ToUpper(region)]; ok {
		return Russian
	}
	if _, ok := russianDefaultLanguages[strings.ToLower(language)]; ok {
		return Russian
	}
	return English
}

func cleanLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if idx := strings.IndexByte(locale, '.'); idx >= 0 {
		locale = locale[:idx]
	}
	if idx := strings.IndexByte(locale, '@'); idx >= 0 {
		locale = locale[:idx]
	}
	return locale
}

func localeLanguage(locale string) string {
	locale = strings.ReplaceAll(locale, "_", "-")
	if idx := strings.Index(locale, "-"); idx >= 0 {
		return locale[:idx]
	}
	return locale
}

func localeRegion(locale string) string {
	locale = strings.ReplaceAll(locale, "_", "-")
	parts := strings.Split(locale, "-")
	for i := len(parts) - 1; i >= 0; i-- {
		part := parts[i]
		if len(part) == 2 {
			return part
		}
	}
	return ""
}
