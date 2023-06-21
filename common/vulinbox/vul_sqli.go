package vulinbox

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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
	router.HandleFunc("/user/id-json", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
		var jsonMap map[string]any
		err := json.Unmarshal([]byte(a), &jsonMap)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		a, ok := jsonMap["id"].(string)
		if !ok {
			writer.Write([]byte("Failed to retrieve the 'id' field"))
			writer.WriteHeader(500)
			return
		}

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
	router.HandleFunc("/user/id-b64-json", func(writer http.ResponseWriter, request *http.Request) {
		var a = request.URL.Query().Get("id")
		decodedB64, err := codec.DecodeBase64(a)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		var jsonMap map[string]any
		err = json.Unmarshal(decodedB64, &jsonMap)
		if err != nil {
			writer.Write([]byte(err.Error()))
			writer.WriteHeader(500)
			return
		}
		a, ok := jsonMap["id"].(string)
		if !ok {
			writer.Write([]byte("Failed to retrieve the 'id' field"))
			writer.WriteHeader(500)
			return
		}

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
