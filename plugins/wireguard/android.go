package wireguard

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"godeb/backend/core"
)

const (
	androidDir      = "/etc/godeb/android"
	androidAPKName  = "godeb-wireguard.apk"
	androidVersion  = "version.txt"
	androidAPKPath  = androidDir + "/" + androidAPKName
	androidVerPath  = androidDir + "/" + androidVersion
	defaultAPKLabel = "godeb-wireguard.apk"
)

type androidClientInfo struct {
	Available    bool   `json:"available"`
	Filename     string `json:"filename"`
	Size         int64  `json:"size,omitempty"`
	Version      string `json:"version,omitempty"`
	Modified     string `json:"modified,omitempty"`
	DownloadPath string `json:"downloadPath"`
	Detail       string `json:"detail,omitempty"`
}

func mountAndroid(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/wireguard/android", handleAndroidInfo)
	mux.HandleFunc("GET /api/plugins/wireguard/android/apk", handleAndroidAPK)
}

func handleAndroidInfo(w http.ResponseWriter, r *http.Request) {
	info := androidClientInfo{
		Filename:     defaultAPKLabel,
		DownloadPath: "/api/plugins/wireguard/android/apk",
	}
	st, err := os.Stat(androidAPKPath)
	if err != nil {
		info.Detail = "Keine APK unter " + androidAPKPath + ". Signed Release-APK dort ablegen."
		core.WriteJSON(w, http.StatusOK, info)
		return
	}
	if st.IsDir() {
		info.Detail = androidAPKPath + " ist ein Verzeichnis"
		core.WriteJSON(w, http.StatusOK, info)
		return
	}
	info.Available = true
	info.Size = st.Size()
	info.Modified = st.ModTime().UTC().Format(time.RFC3339)
	if ver, err := os.ReadFile(androidVerPath); err == nil {
		info.Version = strings.TrimSpace(string(ver))
	}
	core.WriteJSON(w, http.StatusOK, info)
}

func handleAndroidAPK(w http.ResponseWriter, r *http.Request) {
	f, err := os.Open(androidAPKPath)
	if err != nil {
		if os.IsNotExist(err) {
			core.WriteError(w, http.StatusNotFound, "Android-APK nicht vorhanden")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="`+defaultAPKLabel+`"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, defaultAPKLabel, st.ModTime(), f)
}

// AndroidAPKPath is the on-disk location expected by the packaging docs.
func AndroidAPKPath() string {
	return filepath.Clean(androidAPKPath)
}
