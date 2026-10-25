package platform

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (a *Service) publicApplicationRoutes(mux *http.ServeMux) {
	handle := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		d := Doc{}
		var result any
		var err error
		code := 200
		if r.Method != "GET" {
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&d) != nil {
				writeJSON(w, 422, Doc{"error": "Invalid application request"})
				return
			}
			// Browsers must originate application mutations from this application.
			if origin := r.Header.Get("Origin"); origin != "" && origin != a.Config.Origin {
				writeJSON(w, 403, Doc{"error": "Invalid application origin"})
				return
			}
		}
		if r.URL.Path == "/api/public/applications" {
			result, err = a.CreatePublicApplication(r.Context(), d)
			code = 201
		} else {
			id := r.PathValue("id")
			u, o, e := a.publicApplicant(r.Context(), id, r.Header.Get("X-Application-Key"))
			err = e
			if err == nil {
				switch {
				case r.Method == "GET":
					result = publicView(o)
				case r.Method == "PUT":
					if !strings.EqualFold(s(d, "contactEmail"), s(o, "email")) {
						err = fail(422, "The application contact email cannot be changed")
						break
					}
					var updated Doc
					updated, err = a.EditOrganization(r.Context(), u, id, d)
					if err == nil {
						result = publicView(updated)
					}
				case strings.HasSuffix(r.URL.Path, "/submit"):
					var updated Doc
					updated, err = a.Submit(r.Context(), u, id)
					if err == nil {
						result = publicView(updated)
					}
				default:
					if r.PathValue("doc") == "" {
						docs, e := a.Repo.Query(r.Context(), a.Config.ControlTable, "DOCUMENT#"+id, false)
						if e != nil {
							err = e
							break
						}
						if len(docs) >= 8 {
							err = fail(422, "Document limit reached. Contact the service operator for assistance.")
							break
						}
					}
					result, err = a.Document(r.Context(), u, id, r.PathValue("doc"), d)
				}
			}
		}
		if err != nil {
			code = status(err)
			result = Doc{"error": safeError(err)}
		}
		writeJSON(w, code, result)
	}
	mux.HandleFunc("POST /api/public/applications", handle)
	mux.HandleFunc("GET /api/public/applications/{id}", handle)
	mux.HandleFunc("PUT /api/public/applications/{id}", handle)
	mux.HandleFunc("POST /api/public/applications/{id}/submit", handle)
	mux.HandleFunc("POST /api/public/applications/{id}/documents", handle)
	mux.HandleFunc("POST /api/public/applications/{id}/documents/{doc}/complete", handle)
}
