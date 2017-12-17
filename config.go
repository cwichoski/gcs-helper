package main

import (
	"encoding/base64"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"
)

// Config represents the gcs-helper configuration that is loaded from the
// environment.
type Config struct {
	Listen              string   `default:":8080"`
	BucketName          string   `envconfig:"BUCKET_NAME" required:"true"`
	LogLevel            string   `envconfig:"LOG_LEVEL" default:"debug"`
	MapPrefix           string   `envconfig:"MAP_PREFIX"`
	ExtraResourcesToken string   `envconfig:"EXTRA_RESOURCES_TOKEN"`
	MapRegexFilter      string   `envconfig:"MAP_REGEX_FILTER"`
	MapRegexHDFilter    string   `envconfig:"MAP_REGEX_HD_FILTER"`
	MapExtraPrefixes    []string `envconfig:"MAP_EXTRA_PREFIXES"`
	ClientConfig        ClientConfig
	SignConfig          SignConfig
}

// ClientConfig contains configuration for the GCS client communication.
//
// It contains options related to timeouts and keep-alive connections.
type ClientConfig struct {
	Timeout         time.Duration `envconfig:"GCS_CLIENT_TIMEOUT" default:"2s"`
	IdleConnTimeout time.Duration `envconfig:"GCS_CLIENT_IDLE_CONN_TIMEOUT" default:"120s"`
	MaxIdleConns    int           `envconfig:"GCS_CLIENT_MAX_IDLE_CONNS" default:"10"`
}

// SignConfig contains configuration for generating signed URLs in mapped mode.
type SignConfig struct {
	Expiration time.Duration `envconfig:"GCS_SIGNER_EXPIRATION" default:"20m"`
	AccessID   string        `envconfig:"GCS_SIGNER_ACCESS_ID"`
	PrivateKey b64Value      `envconfig:"GCS_SIGNER_PRIVATE_KEY"`
}

type b64Value []byte

func (v *b64Value) Decode(value string) error {
	b, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	*v = b
	return nil
}

// Options returns the SignedURLOptions that should be used for signing object
// URLs.
//
// When URL signing is disabled, it returns two nil values.
func (c *SignConfig) Options() (*storage.SignedURLOptions, error) {
	if c.AccessID == "" || c.PrivateKey == nil {
		return nil, nil
	}
	return &storage.SignedURLOptions{
		Method:         http.MethodGet,
		GoogleAccessID: c.AccessID,
		PrivateKey:     []byte(c.PrivateKey),
		Expires:        time.Now().Add(c.Expiration),
	}, nil
}

func (c Config) logger() *logrus.Logger {
	level, err := logrus.ParseLevel(c.LogLevel)
	if err != nil {
		level = logrus.DebugLevel
	}

	logger := logrus.New()
	logger.Out = os.Stdout
	logger.Level = level
	return logger
}

func loadConfig() (Config, error) {
	var c Config
	err := envconfig.Process("gcs_helper", &c)
	return c, err
}
