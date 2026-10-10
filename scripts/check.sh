#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
files=$(gofmt -l cmd internal)
if [[ -n "$files" ]]; then
  printf 'Run gofmt on:\n%s\n' "$files" >&2
  exit 1
fi
GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go build ./...
if [[ "$(go env GOOS)" == windows ]]; then
  powershell.exe -NoProfile -Command '$tokens = $null; $errors = $null; $null = [System.Management.Automation.Language.Parser]::ParseFile((Join-Path (Get-Location) "uninstall.ps1"), [ref]$tokens, [ref]$errors); if ($errors.Count -ne 0) { $errors | Out-String | Write-Error; exit 1 }'
  go test -race ./...
else
  go test -race ./internal/tunnel ./internal/langcatalog
  go test -race internal/jvm/filter.go internal/jvm/filter_test.go
  go test -race internal/installer/debugger.go internal/installer/debugger_test.go
  printf 'Windows-only runtime tests require Windows.\n'
fi