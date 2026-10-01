package main

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"example.com/course-dns-admin/internal/courseinfra"
)

var page = template.Must(template.New("admin").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Course DNS admin</title><style>
body{font:16px system-ui;max-width:44rem;margin:2rem auto;padding:0 1rem;color:#17202a}form{display:grid;gap:.8rem}label{display:grid;gap:.25rem}input{font:inherit;padding:.55rem;border:1px solid #9aa4ad;border-radius:4px}button{font:inherit;padding:.65rem;background:#145a32;color:white;border:0;border-radius:4px}.result{border-left:4px solid #145a32;padding:.7rem 1rem;background:#f3f7f4}.error{border-color:#a93226;background:#fbf3f2}
</style></head><body><h1>Course DNS admin</h1>
{{if .Message}}<p class="result {{if .IsError}}error{{end}}">{{.Message}}</p>{{end}}
<form method="post" action="/apply">
<label>Course ID<input name="course_id" required value="physics-204"></label>
<label>Managed domain<input name="domain" required placeholder="school.example"></label>
<label>Course host<input name="course_host" required value="physics"></label>
<label>Delivery CNAME target<input name="delivery_target" required placeholder="courses.example.net"></label>
<label>Learner deadline<input name="deadline" type="datetime-local" required></label>
<label>Educator report email<input name="report_email" type="email" required></label>
<button type="submit">Apply DNS records</button></form></body></html>`))

type pageData struct {
	Message string
	IsError bool
}

func main() {
	apiKey := strings.TrimSpace(os.Getenv("INFRAI_API_KEY"))
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	manager := courseinfra.NewCourseDNSManager(courseinfra.NewClient(apiKey))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		render(w, http.StatusOK, pageData{})
	})
	mux.HandleFunc("POST /apply", func(w http.ResponseWriter, r *http.Request) {
		deadline, err := time.Parse("2006-01-02T15:04", r.FormValue("deadline"))
		if err != nil {
			render(w, http.StatusBadRequest, pageData{Message: "Enter a valid learner deadline.", IsError: true})
			return
		}
		result, err := manager.Apply(r.Context(), courseinfra.CourseDNSInput{
			CourseID:       r.FormValue("course_id"),
			Domain:         r.FormValue("domain"),
			CourseHost:     r.FormValue("course_host"),
			DeliveryTarget: r.FormValue("delivery_target"),
			Deadline:       deadline,
			ReportEmail:    r.FormValue("report_email"),
		})
		if err != nil {
			status := http.StatusBadGateway
			var apiErr *courseinfra.APIError
			if errors.As(err, &apiErr) && apiErr.Status >= 400 && apiErr.Status < 500 {
				status = apiErr.Status
			}
			render(w, status, pageData{Message: err.Error(), IsError: true})
			return
		}
		render(w, http.StatusOK, pageData{Message: "Applied " + strings.Join(result.Names, ", ") + " in zone " + result.ZoneID + "."})
	})

	addr := envOr("ADDR", ":8080")
	log.Printf("course DNS admin listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func render(w http.ResponseWriter, status int, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Execute(w, data); err != nil {
		log.Printf("render page: %v", err)
	}
}

func envOr(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
