package langcatalog

import (
	"os"
	"reflect"
	"regexp"
	"sort"
	"testing"

	"github.com/BurntSushi/toml"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var templateArgumentPattern = regexp.MustCompile(`\{\{\s*\.Arg[0-9]+\s*\}\}`)
var declaredKeyPattern = regexp.MustCompile(`Key\s*=\s*"([^"]+)"`)

func TestLanguageCatalogsMatch(t *testing.T) {
	t.Parallel()

	english := loadMessages(t, "../../langs/active.en.toml")
	russian := loadMessages(t, "../../langs/active.ru.toml")

	if !reflect.DeepEqual(sortedKeys(english), sortedKeys(russian)) {
		t.Fatalf(
			"language keys differ\nEnglish: %#v\nRussian: %#v",
			sortedKeys(english),
			sortedKeys(russian),
		)
	}
	for key, englishText := range english {
		russianText := russian[key]
		if englishText == "" || russianText == "" {
			t.Errorf("%s has an empty translation", key)
			continue
		}
		englishArgs := templateArgumentPattern.FindAllString(englishText, -1)
		russianArgs := templateArgumentPattern.FindAllString(russianText, -1)
		sort.Strings(englishArgs)
		sort.Strings(russianArgs)
		if !reflect.DeepEqual(englishArgs, russianArgs) {
			t.Errorf(
				"%s template arguments differ: English %v, Russian %v",
				key,
				englishArgs,
				russianArgs,
			)
		}
	}
}

func TestTunnelCounterTranslations(t *testing.T) {
	t.Parallel()

	bundle := goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	for _, path := range []string{
		"../../langs/active.en.toml",
		"../../langs/active.ru.toml",
	} {
		if _, err := bundle.LoadMessageFile(path); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
	}

	tests := []struct {
		name string
		lang string
		key  string
		data map[string]any
		want string
	}{
		{
			name: "Russian progress",
			lang: "ru",
			key:  "tunnel.progress",
			data: map[string]any{
				"Arg0": 54,
				"Arg1": 55,
				"Arg2": 54,
				"Arg3": 0,
				"Arg4": 1,
			},
			want: "Проверено: 54 из 55 · ответили: 54 · без ответа: 0 · раунд 1",
		},
		{
			name: "Russian complete",
			lang: "ru",
			key:  "tunnel.complete",
			data: map[string]any{
				"Arg0": 54,
				"Arg1": 55,
				"Arg2": 1,
				"Arg3": 1,
			},
			want: "Ответили: 54 · без ответа: 1 · всего: 55 · раунд 1",
		},
		{
			name: "English complete",
			lang: "en",
			key:  "tunnel.complete",
			data: map[string]any{
				"Arg0": 54,
				"Arg1": 55,
				"Arg2": 1,
				"Arg3": 1,
			},
			want: "Replied: 54 · no reply: 1 · total: 55 · round 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := goi18n.NewLocalizer(bundle, tt.lang).Localize(
				&goi18n.LocalizeConfig{
					MessageID:    tt.key,
					TemplateData: tt.data,
				},
			)
			if err != nil {
				t.Fatalf("Localize(): %v", err)
			}
			if got != tt.want {
				t.Fatalf("localized text = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEveryDeclaredKeyHasATranslation(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("../i18n/language.go")
	if err != nil {
		t.Fatalf("read language.go: %v", err)
	}
	messages := loadMessages(t, "../../langs/active.en.toml")
	for _, match := range declaredKeyPattern.FindAllSubmatch(source, -1) {
		key := string(match[1])
		if _, ok := messages[key]; !ok {
			t.Errorf("declared key %q has no translation", key)
		}
	}
}

func loadMessages(t *testing.T, path string) map[string]string {
	t.Helper()

	var messages map[string]string
	if _, err := toml.DecodeFile(path, &messages); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return messages
}

func sortedKeys(messages map[string]string) []string {
	keys := make([]string, 0, len(messages))
	for key := range messages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
