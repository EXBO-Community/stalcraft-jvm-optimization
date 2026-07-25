package jvm

import (
	"slices"
	"testing"
)

func TestFilterArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		original []string
		injected []string
		want     []string
	}{
		{
			name: "replaces configured heap and numeric flags",
			original: []string{
				"-Xmx6g",
				"-Xms6g",
				"-XX:MaxGCPauseMillis=50",
				"-Djava.library.path=runtime/bin",
				"com.example.Main",
				"--gameDir",
				`C:\Games\STALZONE`,
			},
			injected: []string{
				"-Xmx12g",
				"-Xms12g",
				"-XX:MaxGCPauseMillis=100",
			},
			want: []string{
				"-Djava.library.path=runtime/bin",
				"-Xmx12g",
				"-Xms12g",
				"-XX:MaxGCPauseMillis=100",
				"com.example.Main",
				"--gameDir",
				`C:\Games\STALZONE`,
			},
		},
		{
			name: "replaces every injected flag with the same identity",
			original: []string{
				"-XX:-DisableExplicitGC",
				"-Djdk.nio.maxCachedBufferSize=65536",
				"-Dlauncher.option=keep",
				"com.example.Main",
			},
			injected: []string{
				"-XX:+DisableExplicitGC",
				"-Djdk.nio.maxCachedBufferSize=131072",
			},
			want: []string{
				"-Dlauncher.option=keep",
				"-XX:+DisableExplicitGC",
				"-Djdk.nio.maxCachedBufferSize=131072",
				"com.example.Main",
			},
		},
		{
			name: "preserves classpath and application arguments",
			original: []string{
				"-classpath",
				"runtime/*",
				"-Xmx6g",
				"com.example.Main",
				"--username",
				"player",
			},
			injected: []string{"-Xmx8g"},
			want: []string{
				"-classpath",
				"runtime/*",
				"-Xmx8g",
				"com.example.Main",
				"--username",
				"player",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := FilterArgs(tt.original, tt.injected)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("FilterArgs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReplaceArgsPreservesUnrelatedLauncherFlags(t *testing.T) {
	t.Parallel()

	original := []string{
		"-Xmx6g",
		"-XX:+UseG1GC",
		"-Droxy_address_override.ru",
		"-Droxy_address_override.ru=192.0.2.10:29450",
		"-Droxy_address_override.eu=192.0.2.20:29450",
		"-Droxy_address_override.na=192.0.2.30:29450",
		"-Dlauncher.option=keep",
		"com.example.Main",
		"--gameDir",
		`C:\Games\STALZONE`,
	}
	injected := []string{
		"-Droxy_address_override.ru=192.0.2.11:29450",
		"-Droxy_address_override.eu=192.0.2.21:29450",
	}
	want := []string{
		"-Xmx6g",
		"-XX:+UseG1GC",
		"-Droxy_address_override.na=192.0.2.30:29450",
		"-Dlauncher.option=keep",
		"-Droxy_address_override.ru=192.0.2.11:29450",
		"-Droxy_address_override.eu=192.0.2.21:29450",
		"com.example.Main",
		"--gameDir",
		`C:\Games\STALZONE`,
	}

	got := ReplaceArgs(original, injected)
	if !slices.Equal(got, want) {
		t.Fatalf("ReplaceArgs() = %#v, want %#v", got, want)
	}
}
