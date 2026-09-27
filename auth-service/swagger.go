package main

import (
	_ "embed"

	"github.com/gin-gonic/gin"
)

//go:embed docs/openapi.yaml
var openAPISpec []byte

const swaggerHTML = `<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>Auth & User Service API</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({ url: "/swagger/openapi.yaml", dom_id: "#swagger-ui" });
  </script>
</body>
</html>`

// หน้า Swagger UI อยู่ที่ http://localhost:8081/swagger
func registerSwagger(r *gin.Engine) {
	r.GET("/swagger", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(swaggerHTML))
	})
	r.GET("/swagger/openapi.yaml", func(c *gin.Context) {
		c.Data(200, "application/yaml", openAPISpec)
	})
}
