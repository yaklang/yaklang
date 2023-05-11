package vulinbox

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (s *VulinServer) registerSQLinj() {
	var router = s.router
	router.HandleFunc("/user/by-id-safe", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
		i, err := strconv.ParseInt(a, 10, 64)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		u, err := s.database.GetUserById(int(i))
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		d, err := json.Marshal(u)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		writer.Write(d)
		writer.WriteHeader(200)
		return
	})
	router.HandleFunc("/user/id", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
		u, err := s.database.GetUserByIdUnsafe(a)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		d, err := json.Marshal(u)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		writer.Write(d)
		writer.WriteHeader(200)
		return
	})
	router.HandleFunc("/user/name", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("name")
		u, err := s.database.GetUserByUsernameUnsafe(a)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		d, err := json.Marshal(u)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		writer.Write(d)
		writer.WriteHeader(200)
		return
	})
}
