package platform

import (
	"context"
	"encoding/json"
	"io"
	"learning-setu/app/internal/identity"
	"learning-setu/app/internal/store"
	"learning-setu/app/internal/transcription"
	"net/http"
	"strings"
)

func (a *Service) Handler() http.Handler {
	auth := identity.New(a.Config)
	mux := http.NewServeMux()
	type handler func(context.Context, identity.User, Tenant, *http.Request, Doc) (any, error)
	register := func(pattern string, fn handler) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			u, e := auth.Verify(r.Context(), r.Header.Get("Authorization"))
			var t Tenant
			input := Doc{}
			if e != nil {
				e = fail(401, "Sign in required")
			}
			if e == nil && strings.HasPrefix(r.URL.Path, "/api/company/") {
				e = a.Company(r.Context(), u)
			}
			if e == nil && strings.HasPrefix(r.URL.Path, "/api/tenant/") {
				t, e = a.Tenant(r.Context(), u, r.Header.Get("X-Organization-Id"))
			}
			if e == nil && r.Method != "GET" {
				decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
				if x := decoder.Decode(&input); x != nil && x != io.EOF {
					e = fail(422, "Invalid JSON request")
				}
			}
			var result any
			if e == nil {
				result, e = fn(r.Context(), u, t, r, input)
			}
			code := 200
			if e == nil && r.Method == "POST" && (r.URL.Path == "/api/applications" || r.URL.Path == "/api/tenant/students" || r.URL.Path == "/api/tenant/schools" || r.URL.Path == "/api/tenant/assessments" || r.URL.Path == "/api/tenant/privacy" || strings.HasSuffix(r.URL.Path, "/documents") || strings.HasSuffix(r.URL.Path, "/invitations")) {
				code = 201
			}
			if e == nil && strings.Contains(r.URL.Path, "/tenant/assessments/") && r.Method == "POST" {
				code = 202
			}
			if e == nil && strings.HasPrefix(r.URL.Path, "/api/company/organizations/") && strings.HasSuffix(r.URL.Path, "/offboard") && r.Method == "POST" {
				code = 202
			}
			if e != nil {
				code = status(e)
				result = Doc{"error": safeError(e)}
			}
			writeJSON(w, code, result)
		})
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, Doc{"status": "ok"}) })
	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) {
		d := Doc{"activationEmailsEnabled": a.Config.ActivationEmailsEnabled, "integrationsDisabled": a.Config.IntegrationsDisabled, "authMode": a.Config.AuthMode, "cognitoDomain": a.Config.Domain, "cognitoIssuer": a.Config.Issuer, "companyDomain": a.Config.CompanyDomain, "cognitoClient": a.Config.Client, "companyClient": a.Config.CompanyClient, "policies": Policies, "noticeVersion": Notice, "maxAudioBytes": a.Config.MaxAudioBytes, "minAudioSeconds": transcription.MinRecordingSeconds, "maxAudioSeconds": min(a.Config.MaxAudioSeconds, transcription.MaxRecordingSeconds), "retentionMaxDays": a.Config.RetentionMaxDays}
		if a.Config.AuthMode == "local" {
			d["accounts"] = identity.Accounts
		}
		writeJSON(w, 200, d)
	})
	if a.Config.Local() && a.Config.AuthMode == "local" {
		mux.HandleFunc("POST /api/local/session", func(w http.ResponseWriter, r *http.Request) {
			var d Doc
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&d) != nil {
				writeJSON(w, 422, Doc{"error": "Invalid request"})
				return
			}
			token, e := auth.LocalToken(s(d, "account"))
			if e != nil {
				writeJSON(w, 401, Doc{"error": "Unknown synthetic account"})
				return
			}
			writeJSON(w, 200, Doc{"token": token})
		})
	}
	a.publicApplicationRoutes(mux)
	register("POST /api/applications/{id}/claim", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.ClaimApplication(ctx, u, r.PathValue("id"), s(d, "accessKey"))
	})
	register("GET /api/me", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		items, e := a.Repo.Query(ctx, a.Config.ControlTable, "USER#"+u.Sub, true)
		if e != nil {
			return nil, e
		}
		live := []Doc{}
		for _, item := range items {
			member, e := a.Repo.Get(ctx, a.Config.ControlTable, s(item, "pk"), s(item, "sk"))
			if e != nil {
				return nil, e
			}
			o, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", s(item, "orgId"))
			if e != nil {
				return nil, e
			}
			if b(member, "active") && (u.TeamOrg == "" || u.TeamOrg == s(item, "orgId")) {
				member["displayName"] = m(o, "draft")["displayName"]
				member["state"] = o["state"]
				live = append(live, member)
			}
		}
		return Doc{"sub": u.Sub, "email": u.Email, "company": a.Company(ctx, u) == nil, "teamAccount": u.TeamOrg != "", "memberships": live}, nil
	})
	register("GET /api/applications", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.items(ctx, a.Config.ControlTable, "OWNER#"+u.Sub, true)
	})
	register("POST /api/applications", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.CreateOrganization(ctx, u, d)
	})
	register("PUT /api/applications/{id}", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.EditOrganization(ctx, u, r.PathValue("id"), d)
	})
	register("POST /api/applications/{id}/submit", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Submit(ctx, u, r.PathValue("id"))
	})
	register("POST /api/applications/{id}/documents", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Document(ctx, u, r.PathValue("id"), "", d)
	})
	register("POST /api/applications/{id}/documents/{doc}/complete", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Document(ctx, u, r.PathValue("id"), r.PathValue("doc"), d)
	})
	register("POST /api/invitations/accept", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Accept(ctx, u, s(d, "token"))
	})
	register("GET /api/company/organizations", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		items, err := a.Repo.Query(ctx, a.Config.ControlTable, "ORG", false)
		if err != nil {
			return nil, err
		}
		availability, err := a.integrationAvailability(ctx)
		if err != nil {
			return nil, err
		}
		return Doc{"items": items, "integrationAvailability": availability}, nil
	})
	register("POST /api/company/organizations/{id}/{action}", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		if r.PathValue("action") == "integrations" {
			return a.SetIntegrations(ctx, u, r.PathValue("id"), d)
		}
		if r.PathValue("action") == "assessment-processing" {
			return a.SetAssessmentProcessing(ctx, u, r.PathValue("id"), d)
		}
		if r.PathValue("action") == "activation-email" {
			return a.RetryActivationEmail(ctx, u, r.PathValue("id"))
		}
		if r.PathValue("action") == "offboard" {
			return a.Offboard(ctx, u, r.PathValue("id"), d)
		}
		return a.Review(ctx, u, r.PathValue("id"), r.PathValue("action"), s(d, "reason"))
	})
	register("GET /api/company/organizations/{id}/audit", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.items(ctx, a.Config.ControlTable, "AUDIT#"+r.PathValue("id"), false)
	})
	register("GET /api/company/organizations/{id}/documents", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		org, e := a.Repo.Get(ctx, a.Config.ControlTable, "ORG", r.PathValue("id"))
		if e != nil {
			return nil, e
		}
		if org == nil || offboardingState(s(org, "state")) {
			return nil, fail(409, "Organization documents are unavailable during or after offboarding")
		}
		items, e := a.Repo.Query(ctx, a.Config.ControlTable, "DOCUMENT#"+r.PathValue("id"), false)
		if e != nil {
			return nil, e
		}
		out := []Doc{}
		for _, doc := range items {
			if s(doc, "state") == "UPLOADED" {
				url, e := a.Objects.Playback(ctx, a.Config.DocumentBucket, s(doc, "key"), s(doc, "version"))
				if e != nil {
					return nil, e
				}
				doc["url"] = url
				out = append(out, doc)
			}
		}
		return Doc{"items": out}, nil
	})
	register("GET /api/company/jobs", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		items, e := a.Repo.Query(ctx, a.Config.ControlTable, "JOB", false)
		for _, j := range items {
			delete(j, "data")
		}
		return Doc{"items": items}, e
	})
	register("GET /api/tenant/integration-settings", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.integrationSettings(ctx, t.Org)
	})
	register("GET /api/tenant/assessment-settings", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.assessmentSettings(ctx, t.Org)
	})
	register("GET /api/tenant/dashboard", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return Doc{"organization": Doc{"id": t.ID(), "displayName": m(t.Org, "draft")["displayName"], "retentionDays": t.Org["retentionDays"]}, "role": t.Member["role"]}, nil
	})
	register("GET /api/tenant/reading-progress", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.ReadingProgress(ctx, t, r.URL.Query().Get("language"), r.URL.Query().Get("days"))
	})
	register("PUT /api/tenant/organization", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		if e := admin(t); e != nil {
			return nil, e
		}
		if e := required(d, "displayName"); e != nil {
			return nil, e
		}
		m(t.Org, "draft")["displayName"] = d["displayName"]
		return d, a.Repo.Commit(ctx, a.put(t.Org), store.Write{Table: a.Config.ControlTable, Item: t.Member, Check: true}, a.put(audit(t.ID(), u.Sub, "DISPLAY_NAME_CHANGED", "")))
	})
	register("GET /api/tenant/schools", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Schools(ctx, t)
	})
	register("POST /api/tenant/schools", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.CreateSchool(ctx, u, t, d)
	})
	register("GET /api/tenant/passages", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return Doc{"items": Passages}, nil
	})
	register("GET /api/tenant/students", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		items, e := a.Repo.Query(ctx, t.Table, "STUDENT", false)
		out := []Doc{}
		for _, d := range items {
			if !b(d, "deleting") && n(d, "expiresAt") > millis() {
				out = append(out, d)
			}
		}
		return Doc{"items": out}, e
	})
	register("POST /api/tenant/students", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.CreateStudent(ctx, u, t, d)
	})
	register("GET /api/tenant/students/{id}", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Student(ctx, t, r.PathValue("id"))
	})
	register("POST /api/tenant/students/{id}/consent", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Consent(ctx, u, t, r.PathValue("id"), d)
	})
	register("GET /api/tenant/students/{id}/assessments", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		items, e := a.AssessmentHistory(ctx, t, r.PathValue("id"))
		return Doc{"items": items}, e
	})
	register("POST /api/tenant/assessments", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.InitAssessment(ctx, u, t, d)
	})
	register("GET /api/tenant/assessments/{id}", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Assessment(ctx, t, r.PathValue("id"))
	})
	register("POST /api/tenant/assessments/{id}/complete", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.QueueAssessment(ctx, u, t, r.PathValue("id"), false)
	})
	register("POST /api/tenant/assessments/{id}/retry", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.QueueAssessment(ctx, u, t, r.PathValue("id"), true)
	})
	register("GET /api/tenant/assessments/{id}/audio", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Playback(ctx, t, r.PathValue("id"))
	})
	register("GET /api/tenant/members", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		if e := admin(t); e != nil {
			return nil, e
		}
		return a.items(ctx, a.Config.ControlTable, "MEMBER#"+t.ID(), false)
	})
	register("POST /api/tenant/members", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.CreateTeamMember(ctx, u, t, d)
	})
	register("POST /api/tenant/invitations", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Invite(ctx, u, t, d)
	})
	register("PATCH /api/tenant/members/{subject}", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Membership(ctx, u, t, r.PathValue("subject"), d)
	})
	register("POST /api/tenant/privacy", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Privacy(ctx, u, t, d)
	})
	register("GET /api/tenant/privacy", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		if e := admin(t); e != nil {
			return nil, e
		}
		return a.items(ctx, t.Table, "PRIVACY", false)
	})
	register("PUT /api/tenant/retention", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		return a.Retention(ctx, u, t, n(d, "days"))
	})
	register("POST /api/tenant/privacy/{id}/resolve", func(ctx context.Context, u identity.User, t Tenant, r *http.Request, d Doc) (any, error) {
		if e := admin(t); e != nil {
			return nil, e
		}
		if e := required(d, "reason"); e != nil {
			return nil, e
		}
		request, e := a.Repo.Get(ctx, t.Table, "PRIVACY", r.PathValue("id"))
		if e != nil {
			return nil, e
		}
		if s(request, "kind") != "grievance" || s(request, "state") != "OPEN" {
			return nil, fail(409, "Only open grievances can be resolved here")
		}
		request["state"] = "RESOLVED"
		request["resolution"] = d["reason"]
		request["resolvedAt"] = now()
		request["resolvedBy"] = u.Sub
		return request, a.Repo.Commit(ctx, append(a.guards(t), store.Put(t.Table, request), store.Put(t.Table, audit(s(request, "studentId"), u.Sub, "GRIEVANCE_RESOLVED", "")))...)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 404, Doc{"error": "Route not found"}) })
	files := http.FileServer(http.Dir(a.Config.FrontendDir))
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, must-revalidate")
		files.ServeHTTP(w, r)
	}))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		connect := "'self' https://*.s3." + a.Config.Region + ".amazonaws.com " + a.Config.Domain + " " + a.Config.CompanyDomain
		if a.Config.Local() {
			connect += " http://localhost:9000"
		}
		media := connect
		connect += " https://cognito-idp." + a.Config.Region + ".amazonaws.com"
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src "+connect+"; media-src "+media+" blob:; frame-ancestors 'none'; base-uri 'self'")
		mux.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
func (a *Service) items(ctx context.Context, table, pk string, index bool) (Doc, error) {
	items, e := a.Repo.Query(ctx, table, pk, index)
	if items == nil {
		items = []Doc{}
	}
	return Doc{"items": items}, e
}
