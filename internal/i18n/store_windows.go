//go:build windows

package i18n

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const (
	registryPath  = `Software\StalcraftWrapper`
	languageValue = "Language"
)

func Saved() (Language, bool) {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false
	}
	defer key.Close()

	value, _, err := key.GetStringValue(languageValue)
	if err != nil {
		return "", false
	}
	return Normalize(value)
}

func Set(lang Language) error {
	if _, ok := Normalize(string(lang)); !ok {
		return fmt.Errorf("unsupported language: %s", lang)
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, registryPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(languageValue, string(lang)); err != nil {
		return fmt.Errorf("set language: %w", err)
	}
	return nil
}
