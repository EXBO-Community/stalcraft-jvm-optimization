package installer

import "strings"

// ownedDebugger recognizes the complete path-only commands written by the
// split installer and historical wrapper.exe installer. It does not consult
// the filesystem: recovery must work after the old executable was removed.
func ownedDebugger(command string) bool {
	path := strings.TrimSpace(command)
	quoted := strings.HasPrefix(path, `"`)
	if quoted {
		if len(path) < 2 || !strings.HasSuffix(path, `"`) {
			return false
		}
		path = path[1 : len(path)-1]
	}
	ordinaryPath := path
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		ordinaryPath = path[4:]
	}
	if strings.ContainsAny(ordinaryPath, "\"<>|*?\x00\r\n\t") {
		return false
	}

	separator := strings.LastIndexAny(path, `\/`)
	name := path[separator+1:]
	if !strings.EqualFold(name, "service.exe") && !strings.EqualFold(name, "wrapper.exe") {
		return false
	}
	if separator < 0 {
		return true
	}

	// Accept Windows absolute paths and environment-prefixed REG_EXPAND_SZ
	// paths, not command strings such as "cmd /c C:\\old\\service.exe".
	if colon := strings.IndexByte(ordinaryPath, ':'); colon >= 0 {
		if colon != 1 || len(ordinaryPath) < 3 || strings.IndexByte(ordinaryPath[2:], ':') >= 0 {
			return false
		}
		if (ordinaryPath[0] < 'A' || ordinaryPath[0] > 'Z') && (ordinaryPath[0] < 'a' || ordinaryPath[0] > 'z') {
			return false
		}
		if ordinaryPath[2] != '\\' && ordinaryPath[2] != '/' {
			return false
		}
	} else if !strings.HasPrefix(path, `\\`) && !strings.HasPrefix(path, "//") {
		if !strings.HasPrefix(path, "%") {
			return false
		}
		end := strings.IndexByte(path[1:], '%') + 1
		if end <= 1 || end+1 >= len(path) || (path[end+1] != '\\' && path[end+1] != '/') {
			return false
		}
	}

	// Early releases wrote unquoted paths, including paths with spaces.
	// Refuse ambiguous executable-plus-arguments commands.
	if !quoted {
		for i := range len(path) {
			if i+4 < len(path) && strings.EqualFold(path[i:i+4], ".exe") && path[i+4] == ' ' {
				return false
			}
			if path[i] == ' ' && i+1 < len(path) && (path[i+1] == '\\' || path[i+1] == '/') {
				return false
			}
		}
	}
	return true
}
