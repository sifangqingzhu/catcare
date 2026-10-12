package config

import (
	"fmt"
	"github.com/joho/godotenv"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

type Config struct {
	TrainingTimezone                                        *time.Location
	DSN, Secret, Address, Origin, FrontendDir, DemoPassword string
	BootstrapUsername, BootstrapPassword                    string
	TrustedProxies                                          []string
	Secure, Demo                                            bool
}

func Load() (Config, error) {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	c := Config{DSN: os.Getenv("DATABASE_URL"), Secret: os.Getenv("SESSION_SECRET"), Origin: os.Getenv("APP_ORIGIN"), DemoPassword: os.Getenv("DEMO_PASSWORD"), FrontendDir: os.Getenv("FRONTEND_DIR")}
	c.BootstrapUsername = os.Getenv("BOOTSTRAP_USERNAME")
	c.BootstrapPassword = os.Getenv("BOOTSTRAP_PASSWORD")
	zone := os.Getenv("APP_TIMEZONE")
	if zone == "" {
		zone = "Asia/Tokyo"
	}
	var zoneError error
	c.TrainingTimezone, zoneError = time.LoadLocation(zone)
	if zoneError != nil {
		return c, fmt.Errorf("invalid APP_TIMEZONE")
	}
	if value := os.Getenv("TRUSTED_PROXIES"); value != "" {
		c.TrustedProxies = strings.Split(value, ",")
	}
	c.Secure = os.Getenv("APP_ENV") == "production"
	c.Demo = os.Getenv("ENABLE_DEMO") == "true" && !c.Secure
	host := os.Getenv("APP_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return c, fmt.Errorf("invalid API_PORT")
	}
	c.Address = host + ":" + port
	if c.DSN == "" || len(c.Secret) < 32 || c.Origin == "" {
		return c, fmt.Errorf("DATABASE_URL, SESSION_SECRET (32+ chars), APP_ORIGIN are required in .env")
	}
	if c.Secure && !strings.HasPrefix(c.Origin, "https://") {
		return c, fmt.Errorf("production APP_ORIGIN must use HTTPS")
	}
	if c.Demo && len(c.DemoPassword) < 12 {
		return c, fmt.Errorf("DEMO_PASSWORD (12+ chars) required when ENABLE_DEMO=true")
	}
	if (c.BootstrapUsername == "") != (c.BootstrapPassword == "") || (c.BootstrapUsername != "" && (len(c.BootstrapPassword) < 8 || len(c.BootstrapPassword) > 72)) {
		return c, fmt.Errorf("BOOTSTRAP_USERNAME and an 8–72 byte BOOTSTRAP_PASSWORD must be supplied together")
	}
	return c, nil
}
