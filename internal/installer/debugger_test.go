package installer

import "testing"

func TestOwnedDebugger(t *testing.T) {
	cases := []struct {
		name    string
		command string
		owned   bool
	}{
		{"current missing executable", `"C:\removed\service.exe"`, true},
		{"current path with spaces", `"C:\Program Files\jvm_wrapper\service.exe"`, true},
		{"legacy quoted", `"D:\old\wrapper.exe"`, true},
		{"legacy unquoted spaces", `C:\Program Files\old\wrapper.exe`, true},
		{"case insensitive", `"C:\old\WRAPPER.EXE"`, true},
		{"UNC path", `"\\server\share\old\service.exe"`, true},
		{"extended path", `"\\?\C:\old\service.exe"`, true},
		{"forward separators", `C:/old/service.exe`, true},
		{"expandable path", `"%LOCALAPPDATA%\old\wrapper.exe"`, true},
		{"basename", `service.exe`, true},
		{"surrounding whitespace", " \t\"C:\\old\\wrapper.exe\" \r\n", true},
		{"foreign debugger", `"C:\tools\debugger.exe"`, false},
		{"similar foreign basename", `"C:\tools\notservice.exe"`, false},
		{"renamed executable", `"C:\old\wrapper.exe.bak"`, false},
		{"foreign quoted command", `"C:\Windows\cmd.exe" /c "C:\old\service.exe"`, false},
		{"foreign unquoted command", `C:\Windows\cmd.exe /c C:\old\service.exe`, false},
		{"foreign extensionless command", `C:\Windows\cmd /c \old\service.exe`, false},
		{"relative foreign command", `cmd /c \old\service.exe`, false},
		{"owned command with arguments", `"C:\old\service.exe" --other-program`, false},
		{"unquoted command with arguments", `C:\old\service.exe --other-program`, false},
		{"unclosed quote", `"C:\old\wrapper.exe`, false},
		{"extra quote", `"C:\old\"wrapper.exe"`, false},
		{"invalid path operator", `C:\tools|\service.exe`, false},
		{"invalid drive path", `1:\old\service.exe`, false},
		{"unterminated variable", `%LOCALAPPDATA\old\service.exe`, false},
		{"relative path", `..\other\service.exe`, false},
		{"embedded control", "C:\\old\t\\service.exe", false},
		{"empty value", ``, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ownedDebugger(tc.command); got != tc.owned {
				t.Errorf("ownedDebugger(%q) = %v; want %v", tc.command, got, tc.owned)
			}
		})
	}
}
