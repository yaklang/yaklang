package example

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func handler(w http.ResponseWriter, r *http.Request) {
	// 从查询参数获取 URL
	url := r.URL.Query().Get("url")

	// 发送请求
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, "Error fetching URL", http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Error reading response", http.StatusInternalServerError)
		return
	}

	// 返回响应内容
	w.Write(body)
}

func main() {
	http.HandleFunc("/", handler)
	fmt.Println("Server is running on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Failed to start server:", err)
		os.Exit(1)
	}
}
