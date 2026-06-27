package main

import "github.com/gin-gonic/gin"

func main() {
	r := gin.Default()
	r.Get("/api/products", listProducts)
	r.Post("/api/products", createProduct)
	r.Delete("/api/products/:id", deleteProduct)
}
