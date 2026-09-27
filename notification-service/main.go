package main

import (
	"log"
	"os"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	connectDB(os.Getenv("DATABASE_URL"))
	connectAuthService(os.Getenv("AUTH_SERVICE_URL"))

	keycloakURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")

	// โหลด public key ของ Keycloak ไว้ตรวจลายเซ็นของ token (library จะโหลดใหม่เองเมื่อ key เปลี่ยน)
	jwks, err := keyfunc.NewDefault([]string{keycloakURL + "/realms/" + realm + "/protocol/openid-connect/certs"})
	if err != nil {
		log.Fatalf("cannot load keycloak keys: %v", err)
	}

	// ใน Docker service เรียก Keycloak ผ่าน http://keycloak:8080 แต่ token ที่ผู้ใช้ได้มา
	// ออกผ่าน http://localhost:8080 จึงต้องตั้ง issuer แยกได้
	issuer := os.Getenv("KEYCLOAK_ISSUER")
	if issuer == "" {
		issuer = keycloakURL + "/realms/" + realm
	}

	r := setupRouter(
		AuthMiddleware(jwks.Keyfunc, issuer),
		InternalKeyMiddleware(os.Getenv("INTERNAL_API_KEY")),
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}
	r.Run(":" + port)
}
