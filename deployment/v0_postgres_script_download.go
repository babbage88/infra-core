package deployment

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
)

func downloadDnUserScriptsHandler(w http.ResponseWriter, r *http.Request) {
	var requestData PgSetupScriptsRequest
	if err := json.NewDecoder(r.Body).Decode(&requestData); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	scripts := GenerateSqlScript(requestData.DbHostName,
		requestData.SuperuserUsername,
		requestData.SuperuserPassword,
		requestData.ServiceUsername,
		requestData.ServicePassword,
		requestData.DatabaseName,
		requestData.DbPort,
		defaultAppSchemaName,
	)

	// Create zip buffer
	buf := new(bytes.Buffer)
	zipWriter := zip.NewWriter(buf)

	files := map[string]string{
		"create_dev_db.sh": scripts.ShellScript,
		"pg_create_db.sql": scripts.PgCreateScript,
		"pg_app_db.sql":    scripts.AppDbSetupScript,
	}

	for name, content := range files {
		f, err := zipWriter.Create(name)
		if err != nil {
			http.Error(w, "error creating zip file", http.StatusInternalServerError)
			return
		}
		_, err = f.Write([]byte(content))
		if err != nil {
			http.Error(w, "error writing to zip", http.StatusInternalServerError)
			return
		}
	}

	if err := zipWriter.Close(); err != nil {
		http.Error(w, "error finalizing zip", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="pg_setup_scripts.zip"`)
	w.Write(buf.Bytes())
}

func DownloadDbUserScriptsHandler() func(w http.ResponseWriter, r *http.Request) {
	return downloadDnUserScriptsHandler
}
