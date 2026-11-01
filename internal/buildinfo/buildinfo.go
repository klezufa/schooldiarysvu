// Package buildinfo сообщает версию сборки и имя экземпляра процесса.
package buildinfo

import (
	"os"
	"runtime/debug"
	"strconv"
)

// Version можно задать при сборке: -ldflags "-X schooldiarysvu/internal/buildinfo.Version=1.2.3".
var Version = ""

// Current возвращает заданную версию, иначе короткий хеш коммита из сведений сборки, иначе "dev".
func Current() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				return setting.Value[:7]
			}
		}
	}
	return "dev"
}

// Instance — имя экземпляра: хост и номер процесса.
func Instance() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown"
	}
	return host + "-" + strconv.Itoa(os.Getpid())
}
