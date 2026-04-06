# stalcraft-wrapper

[![en](https://img.shields.io/badge/lang-English-blue)](README.en.md)

JVM-враппер для STALCRAFT. Автоматически оптимизирует настройки Java под ваше железо для лучшей производительности.

## Что делает

- Подбирает оптимальные настройки Java (память, сборщик мусора, потоки) под ваш ПК
- Повышает приоритет процесса игры
- Устанавливается один раз — работает автоматически при каждом запуске
- Файлы игры не затрагиваются

## Установка

1. Скачайте `wrapper.exe` из [Releases](../../releases)
2. Положите куда угодно
3. Запустите от имени администратора

Откроется меню:

```
  > Install
    Uninstall
    Status
    Exit
```

Стрелками выберите **Install**, нажмите Enter. Готово.

Поддерживаются обе версии игры:
- `stalcraft.exe` (основной лаунчер)
- `stalcraftw.exe` (Steam)

## Удаление

Запустите `wrapper.exe` от админа и выберите **Uninstall**.

## Large Pages (необязательно)

Для дополнительной производительности можно включить large pages:

1. Откройте `secpol.msc`
2. Локальные политики &rarr; Назначение прав пользователя &rarr; Блокировка страниц в памяти
3. Добавьте своего пользователя, перезагрузитесь

Враппер определит это сам и включит автоматически.

## Требования

- Windows 10/11
- Права администратора (для установки/удаления)

---

## Техническая информация

### Механизм работы

Враппер использует [IFEO](https://learn.microsoft.com/en-us/previous-versions/windows/desktop/xperf/image-file-execution-options) для перехвата запуска игры. При запуске `stalcraft.exe` / `stalcraftw.exe` Windows перенаправляет вызов через враппер, который:

1. Определяет железо: RAM, CPU, large pages (`GlobalMemoryStatusEx`, `GetLargePageMinimum`)
2. Генерирует JVM-флаги под текущую конфигурацию
3. Убирает конфликтующие флаги из оригинальных аргументов лаунчера
4. Запускает процесс напрямую через `ntdll!NtCreateUserProcess`, обходя повторный IFEO-перехват
5. Устанавливает повышенный приоритет памяти и I/O через `NtSetInformationProcess`
6. Завершается после появления первого видимого окна игры

### Обход IFEO

Процесс создаётся через `NtCreateUserProcess` (ntdll) напрямую, минуя `CreateProcessInternalW` (kernel32), где происходит проверка IFEO. Дополнительно выставляется бит `IFEOSkipDebugger` в `PS_CREATE_INFO`.

### Динамический подбор флагов

| Параметр | Формула |
|----------|---------|
| Heap | 50% свободной RAM, floor 25% общей, cap min(16g, 75% общей) |
| ParallelGCThreads | ядра - 2, мин. 2 |
| ConcGCThreads | parallel / 4, мин. 1 |
| G1HeapRegionSize | 4m / 8m / 16m / 32m по размеру heap |
| Metaspace | 128m / 256m / 512m по размеру heap |
| CodeCache | heap/16, в пределах 128-512m |
| SurvivorRatio | 32 (&le;4 ядер) / 8 (>4 ядер) |
| Large Pages | автоматически при наличии привилегии |

На системах с &le;8GB RAM флаги не инжектируются.

### CLI

```
wrapper.exe --install     # установить IFEO-хук
wrapper.exe --status      # проверить статус
wrapper.exe --uninstall   # удалить IFEO-хук
```

### Сборка

```
cd wrapper
go build -o wrapper.exe -ldflags="-s -w" .
```

## Лицензия

MIT
