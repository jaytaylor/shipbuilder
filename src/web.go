package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gorilla/pat"
)

type (
	JsonResponse struct {
		Meta	map[string]interface{}
		Objects interface{}
	}
)

func ToJson(obj interface{}) ([]byte, error) {
	var buffer bytes.Buffer
	err := json.NewEncoder(&buffer).Encode(obj)
	if err != nil {
		return []byte{}, err
	}
	return buffer.Bytes(), nil
}

func httpResponse(w http.ResponseWriter, code int, body []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(code)
	w.Write(body)
}

func jsonResponse(w http.ResponseWriter, code int, obj interface{}) {
	w.Header().Set("Content-Type", "application/json")
	response := JsonResponse{Meta: map[string]interface{}{"foo": "Bar"}, Objects: obj}
	body, err := ToJson(response)
	if err != nil {
		w.WriteHeader(500)
		body = []byte(fmt.Sprint(`{"error": "fatal - object conversion to json failed, reason: %v"}`, strings.Replace(err.Error(), `"`, `\"`, -1)))
	} else {
		w.WriteHeader(code)
	}
	w.Write(body)
}

func jsonErrorResponse(w http.ResponseWriter, err error) {
	jsonResponse(w, 503, map[string]string{"error": err.Error()}) // NB: 503 = Service Unavailable.
}

// Takes a SB Server reference and returns a handler.
func apiGetAppHandler(sbServer *Server) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		appName := req.URL.Query().Get(":name")
		application, err := sbServer.Application(appName)
		if err != nil {
			jsonErrorResponse(w, err)
			return
		}
		jsonResponse(w, 200, application)
	}
}

// Takes a SB Server reference and returns a handler.
func apiGetAppsHandler(sbServer *Server) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		applications, err := sbServer.Applications()
		if err != nil {
			jsonErrorResponse(w, err)
			return
		}
		jsonResponse(w, 200, applications)
	}
}

// Found at: http://stackoverflow.com/questions/16682797/how-to-convert-a-rune-to-unicode-style-string-like-u554a-in-golang
func RuneToAscii(r rune) string {
	if r < 128 {
		return string(r)
	} else {
		return "\\u" + strconv.FormatInt(int64(r), 16)
	}
}

// Returns the path to the directory where ShipBuilder is running.
func ProjectRoot() (string, error) {
	path, err := filepath.Abs(os.Args[0])
	if err != nil {
		return "", err
	}
	return path[0:strings.LastIndex(path, RuneToAscii(os.PathSeparator))], nil
}

func WebRoot() string {
	projectPath, err := ProjectRoot()
	if err != nil {
		panic(err)
	}
	return projectPath + "/webroot"
}

// Takes a SB Server reference and returns a handler.
/*func webHandler(sbServer *Server) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, req *http.Request) {
		path, err := ProjectRootPath()
		if err != nil {
			jsonErrorResponse(w, err)
			return
		}

		content, err := ioutil.ReadFile(path + "/webroot/index.html")
		if err != nil {
			jsonErrorResponse(w, err)
			return
		}
		httpResponse(w, 200, content, "text/html; charset=utf-8")
	}
}*/

func indexHandler(w http.ResponseWriter, req *http.Request) {
	content, err := ioutil.ReadFile(WebRoot() + "/index.html")
	if err != nil {
		jsonErrorResponse(w, err)
		return
	}
	httpResponse(w, 200, content, "text/html")
}

func apiRouting(sbServer *Server) *pat.Router {
	r := pat.New()

	r.Get("/api/v1/app/{name}", apiGetAppHandler(sbServer))
	r.Get("/api/v1/app", apiGetAppsHandler(sbServer))

	for _, path := range []string{
		//"/",
		"/web/app",
		"/web/app/{name}",
	} {
		r.Get(path, indexHandler)
	}

	// NB: This is a way to server static files with gorilla/pat or gorilla/mux.
	r.PathPrefix("/web").Handler(http.StripPrefix("/web/", http.FileServer(http.Dir(WebRoot()))))

	//// Redirect root URL to /web.
	//r.Get("/", func(w http.ResponseWriter, req *http.Request) {
	//	http.Redirect(w, req, "/web/index.html", 301)
	//})

	return r
}

func StartWebServer(sbServer *Server, listenPort int) {
	listenPortStr := strconv.Itoa(listenPort)
	fmt.Println("Starting HTTP server on port " + listenPortStr + " with static-files served from " + WebRoot())
	http.Handle("/", apiRouting(sbServer))
	http.ListenAndServe(":"+listenPortStr, nil)
}
