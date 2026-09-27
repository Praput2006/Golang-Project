package config

import "os"

type Config struct {
	Port           string
	DatabaseURL    string
	KeycloakURL    string
	KeycloakRealm  string
	KeycloakIssuer string // เว้นว่าง = ไม่ตรวจ issuer (สะดวกตอน dev ที่ hostname ไม่ตรงกัน)
}

func Load() Config {
	return Config{
		Port:           getEnv("PORT", "8082"),
		DatabaseURL:    getEnv("DATABASE_URL", "host=localhost user=postgres password=postgres dbname=facility_db port=5432 sslmode=disable TimeZone=Asia/Bangkok"),
		KeycloakURL:    getEnv("KEYCLOAK_URL", "http://localhost:8080"),
		KeycloakRealm:  getEnv("KEYCLOAK_REALM", "facility-booking"),
		KeycloakIssuer: os.Getenv("KEYCLOAK_ISSUER"),
	}
}

func (c Config) JWKSURL() string {
	return c.KeycloakURL + "/realms/" + c.KeycloakRealm + "/protocol/openid-connect/certs"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
