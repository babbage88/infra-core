package cert_renew

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func Renewcert_renew() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Received POST request for Cert Renewal")
		var req CertDnsRenewReq
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			slog.Error("Failed to decode request body", slog.String("Error", err.Error()))
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		if err := req.Validate(); err != nil {
			slog.Error("Invalid certificate renewal request", slog.String("error", err.Error()))
			http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
			return
		}

		req.NormalizeTimeout()

		slog.Info("Decoded request body", slog.String("DomainName", req.DomainNames[0]))

		certInfo, err := req.Renew()
		if err != nil {
			slog.Error("error renewing cert", slog.String("error", err.Error()))
			http.Error(w, "Failed renewing certificate: "+err.Error(), http.StatusInternalServerError)
			return
		}

		slog.Info("Renewal command executed")
		slog.Info("Marshaling JSON response", slog.String("DomainName", certInfo.DomainNames[0]))

		jsonResponse, err := json.Marshal(certInfo)
		if err != nil {
			slog.Error("Failed to marshal JSON response", slog.String("Error", err.Error()))
			http.Error(w, "Failed to marshal JSON response: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonResponse)
		slog.Info("Response sent successfully")
	}
}
