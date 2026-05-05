package site

import (
	"archive/zip"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var (
	timeZoneOptionsOnce  sync.Once
	timeZoneOptionsCache []string
)

// TimeZoneOptions returns IANA time zone names suitable for the settings UI.
func TimeZoneOptions(current string) []string {
	timeZoneOptionsOnce.Do(func() {
		timeZoneOptionsCache = discoverTimeZoneOptions()
	})

	options := append([]string(nil), timeZoneOptionsCache...)
	current = NormalizeTimeZone(current)
	if current != "" && !containsString(options, current) {
		options = append(options, current)
		sort.Strings(options)
	}
	return options
}

func discoverTimeZoneOptions() []string {
	seen := map[string]bool{}
	add := func(name string) {
		name = cleanTimeZoneName(name)
		if name == "" || seen[name] || !ValidTimeZone(name) {
			return
		}
		seen[name] = true
	}

	for _, name := range fallbackTimeZones {
		add(name)
	}
	for _, source := range timeZoneSources() {
		collectTimeZoneSource(source, add)
	}

	options := make([]string, 0, len(seen))
	for name := range seen {
		options = append(options, name)
	}
	sort.Strings(options)
	return options
}

func timeZoneSources() []string {
	var sources []string
	if zoneinfo := strings.TrimSpace(os.Getenv("ZONEINFO")); zoneinfo != "" {
		sources = append(sources, zoneinfo)
	}
	if goroot := runtime.GOROOT(); goroot != "" {
		sources = append(sources, filepath.Join(goroot, "lib", "time", "zoneinfo.zip"))
	}
	sources = append(sources,
		"/usr/share/zoneinfo",
		"/usr/share/lib/zoneinfo",
		"/usr/lib/locale/TZ",
	)
	return sources
}

func collectTimeZoneSource(source string, add func(string)) {
	info, err := os.Stat(source)
	if err != nil {
		return
	}
	if info.IsDir() {
		collectTimeZoneDir(source, add)
		return
	}
	if strings.EqualFold(filepath.Ext(source), ".zip") {
		collectTimeZoneZip(source, add)
	}
}

func collectTimeZoneDir(root string, add func(string)) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		add(filepath.ToSlash(rel))
		return nil
	})
}

func collectTimeZoneZip(path string, add func(string)) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		add(file.Name)
	}
}

func cleanTimeZoneName(name string) string {
	name = strings.TrimSpace(filepath.ToSlash(name))
	name = strings.TrimPrefix(name, "./")
	if name == "" || strings.HasSuffix(name, "/") || strings.Contains(name, "..") {
		return ""
	}
	if strings.HasPrefix(name, "posix/") || strings.HasPrefix(name, "right/") || strings.HasPrefix(name, "SystemV/") {
		return ""
	}

	switch name {
	case "Factory", "localtime", "iso3166.tab", "leap-seconds.list", "leapseconds", "tzdata.zi", "zone.tab", "zone1970.tab":
		return ""
	case "UTC", "Etc/UTC":
		return name
	}
	if strings.HasPrefix(name, "Etc/GMT") {
		return ""
	}
	if !strings.Contains(name, "/") {
		return ""
	}
	return name
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

var fallbackTimeZones = []string{
	"UTC",
	"Africa/Cairo",
	"Africa/Johannesburg",
	"Africa/Lagos",
	"Africa/Nairobi",
	"America/Anchorage",
	"America/Argentina/Buenos_Aires",
	"America/Bogota",
	"America/Caracas",
	"America/Chicago",
	"America/Denver",
	"America/Guatemala",
	"America/Halifax",
	"America/Lima",
	"America/Los_Angeles",
	"America/Mexico_City",
	"America/New_York",
	"America/Phoenix",
	"America/Santiago",
	"America/Sao_Paulo",
	"America/St_Johns",
	"America/Toronto",
	"America/Vancouver",
	"Asia/Almaty",
	"Asia/Amman",
	"Asia/Baghdad",
	"Asia/Bangkok",
	"Asia/Dhaka",
	"Asia/Dubai",
	"Asia/Ho_Chi_Minh",
	"Asia/Hong_Kong",
	"Asia/Jakarta",
	"Asia/Jerusalem",
	"Asia/Karachi",
	"Asia/Kathmandu",
	"Asia/Kolkata",
	"Asia/Kuala_Lumpur",
	"Asia/Manila",
	"Asia/Riyadh",
	"Asia/Seoul",
	"Asia/Shanghai",
	"Asia/Singapore",
	"Asia/Taipei",
	"Asia/Tokyo",
	"Asia/Ulaanbaatar",
	"Asia/Yangon",
	"Atlantic/Reykjavik",
	"Australia/Adelaide",
	"Australia/Brisbane",
	"Australia/Darwin",
	"Australia/Melbourne",
	"Australia/Perth",
	"Australia/Sydney",
	"Europe/Amsterdam",
	"Europe/Athens",
	"Europe/Berlin",
	"Europe/Brussels",
	"Europe/Bucharest",
	"Europe/Dublin",
	"Europe/Helsinki",
	"Europe/Istanbul",
	"Europe/Lisbon",
	"Europe/London",
	"Europe/Madrid",
	"Europe/Moscow",
	"Europe/Paris",
	"Europe/Rome",
	"Europe/Stockholm",
	"Europe/Vienna",
	"Europe/Warsaw",
	"Europe/Zurich",
	"Pacific/Auckland",
	"Pacific/Guam",
	"Pacific/Honolulu",
	"Pacific/Pago_Pago",
}
