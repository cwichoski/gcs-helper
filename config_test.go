package main

import (
	"math"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/sirupsen/logrus"
)

func TestLoadConfig(t *testing.T) {
	setEnvs(map[string]string{
		"GCS_HELPER_LISTEN":              "0.0.0.0:3030",
		"GCS_HELPER_BUCKET_NAME":         "some-bucket",
		"GCS_HELPER_LOG_LEVEL":           "info",
		"GCS_HELPER_MAP_PREFIX":          "/map/",
		"GCS_HELPER_MAP_REGEX_FILTER":    `(240|360|424|480|720|1080)p(\.mp4|[a-z0-9_-]{37}\.(vtt|srt))$`,
		"GCS_HELPER_MAP_REGEX_HD_FILTER": `((720|1080)p\.mp4)|(\.(vtt|srt))$`,
		"GCS_HELPER_MAP_EXTRA_PREFIXES":  "subtitles/,mp4s/",
		"GCS_CLIENT_TIMEOUT":             "60s",
		"GCS_CLIENT_IDLE_CONN_TIMEOUT":   "3m",
		"GCS_CLIENT_MAX_IDLE_CONNS":      "16",
		"GCS_SIGNER_EXPIRATION":          "10m",
		"GCS_SIGNER_ACCESS_ID":           "access",
		"GCS_SIGNER_PRIVATE_KEY":         "c2VjcmV0IQ==",
	})
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	expectedConfig := Config{
		BucketName:       "some-bucket",
		Listen:           "0.0.0.0:3030",
		LogLevel:         "info",
		MapPrefix:        "/map/",
		MapExtraPrefixes: []string{"subtitles/", "mp4s/"},
		MapRegexFilter:   `(240|360|424|480|720|1080)p(\.mp4|[a-z0-9_-]{37}\.(vtt|srt))$`,
		MapRegexHDFilter: `((720|1080)p\.mp4)|(\.(vtt|srt))$`,
		ClientConfig: ClientConfig{
			IdleConnTimeout: 3 * time.Minute,
			MaxIdleConns:    16,
			Timeout:         time.Minute,
		},
		SignConfig: SignConfig{
			AccessID:   "access",
			PrivateKey: []byte("secret!"),
			Expiration: 10 * time.Minute,
		},
	}
	if !reflect.DeepEqual(config, expectedConfig) {
		t.Errorf("wrong config returned\nwant %#v\ngot  %#v", expectedConfig, config)
	}
}

func TestLoadConfigDefaultValues(t *testing.T) {
	setEnvs(map[string]string{"GCS_HELPER_BUCKET_NAME": "some-bucket"})
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	expectedConfig := Config{
		BucketName: "some-bucket",
		Listen:     ":8080",
		LogLevel:   "debug",
		ClientConfig: ClientConfig{
			IdleConnTimeout: 120 * time.Second,
			MaxIdleConns:    10,
			Timeout:         2 * time.Second,
		},
		SignConfig: SignConfig{
			Expiration: 20 * time.Minute,
		},
	}
	if !reflect.DeepEqual(config, expectedConfig) {
		t.Errorf("wrong config returned\nwant %#v\ngot  %#v", expectedConfig, config)
	}
}

func TestConfigLogger(t *testing.T) {
	setEnvs(map[string]string{"GCS_HELPER_BUCKET_NAME": "some-bucket", "GCS_HELPER_LOG_LEVEL": "info"})
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	logger := config.logger()
	if logger.Out != os.Stdout {
		t.Errorf("wrong log output, want os.Stdout, got %#v", logger.Out)
	}
	if logger.Level != logrus.InfoLevel {
		t.Errorf("wrong log leve, want InfoLevel (%v), got %v", logrus.InfoLevel, logger.Level)
	}
}

func TestConfigLoggerInvalidLevel(t *testing.T) {
	setEnvs(map[string]string{"GCS_HELPER_BUCKET_NAME": "some-bucket", "GCS_HELPER_LOG_LEVEL": "dunno"})
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	logger := config.logger()
	if logger.Out != os.Stdout {
		t.Errorf("wrong log output, want os.Stdout, got %#v", logger.Out)
	}
	if logger.Level != logrus.DebugLevel {
		t.Errorf("wrong log leve, want DebugLevel (%v), got %v", logrus.DebugLevel, logger.Level)
	}
}

func TestLoadConfigValidation(t *testing.T) {
	setEnvs(nil)
	config, err := loadConfig()
	if err == nil {
		t.Error("unexpected <nil> error")
	}
	expectedConfig := Config{Listen: ":8080"}
	if !reflect.DeepEqual(config, expectedConfig) {
		t.Errorf("wrong config returned\nwant %#v\ngot  %#v", expectedConfig, config)
	}
}

func TestSignConfigOptions(t *testing.T) {
	var tests = []struct {
		name            string
		input           SignConfig
		expectedOptions *storage.SignedURLOptions
		expectError     bool
	}{
		{
			"valid config",
			SignConfig{AccessID: testdataAccessKeyID, PrivateKey: []byte(testdataPrivateKey), Expiration: time.Minute},
			&storage.SignedURLOptions{
				Method:         http.MethodGet,
				Expires:        time.Now().Add(time.Minute),
				GoogleAccessID: "testing@gcs-helper-test.iam.gserviceaccount.com",
				PrivateKey:     []byte(testdataPrivateKey),
			},
			false,
		},
		{
			"no config",
			SignConfig{Expiration: time.Hour},
			nil,
			false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, err := test.input.Options()
			if test.expectError == (err == nil) {
				t.Errorf("mismatch in error\nexpectError: %v\ngot error: %#v", test.expectError, err)
			}
			if test.expectedOptions != nil {
				diff := math.Abs(float64(opts.Expires.Sub(test.expectedOptions.Expires)))
				if time.Duration(diff) > time.Second {
					t.Errorf("expiration is off by more than one second\nwant: %s\ngot:  %s", test.expectedOptions.Expires, opts.Expires)
				}
				opts.Expires = test.expectedOptions.Expires
			}
			if !reflect.DeepEqual(opts, test.expectedOptions) {
				t.Errorf("wrong options returned\nwant %#v\ngot  %#v", test.expectedOptions, opts)
			}
		})
	}
}

func setEnvs(envs map[string]string) {
	os.Clearenv()
	for name, value := range envs {
		os.Setenv(name, value)
	}
}

const (
	testdataAccessKeyID = "testing@gcs-helper-test.iam.gserviceaccount.com"
	testdataPrivateKey  = "-----BEGIN PRIVATE KEY-----\nMIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCkYNVWeHUkggRk\nJrfj1/vcJvZguHIl2h2GXxKHEJ2A0yMLbbPfEyrPA3R8dFXHb3m181H0X6294XJb\nJsJXN0IzXSf6FzBun7gj6tscpQxMO5fOAah5hq6qDpHAU+aj3gKMCBPIOs1nxtrr\nyeDx7zgxU7DzNtqs1BnDytWI/mj3QXUc9wlRQXevOVzzpdxIFIJGLonqRWbzinEq\ncMokVm1y1uXj1RBn5slsATdKShfoHSg1c68qV9EaolJKVhx1ZsnHbGKP90DhXTXT\ntXVkZem0yD2EaXYWtlL7LKUT/Xw/E4vWfASgMynaZI+b9/FDeAmt3ZkixA5ewPED\n+UQfzBT7AgMBAAECggEAIwRBzibhBYLw/ojE+bOEArUGHTqNjoS1b2+HWeBvPQc9\nWuzmuWmy3+CjivOZZl/X9Ku91KohL+b73nEWS1AJOTnqDzurZJV/u58HSEXcpcy4\nHPl7c0/+m1l5MRhudJARyNTbqfbk1OumrT4XPlKwjMmAU39m/BQ+3Nezv3g60hj3\nBXoIAJayWDYSPAgSdwwKI2SNS4re/Qwb16YfCae3ozOt4ZZyGFPp25YiXUXrPR/4\nMtSdTXyOXazam9bj8KRsmfR8W55K8aBnvyiQYkc49LLTDB/G6TAyhaH4+HzeXsQ+\nZLGR93Wr7JZOymLSgKVyjRBLV5v0lCjlbMe8DgGTaQKBgQDcCz0evugR02qM60VT\nxJY8qmVl9RpZwAd4r94i2gVDtE6/t+WUhHh4wJjWdpmqk87gIwRJrZ7b/vkU3qi2\nwzF02D4YSmIk7NwVqAHwQ56g7DWW1ozPh53pXWJxP6bsV31jGRfr15VLlYmuHu1h\nYk4cG0MkPXUoCM9ChYcUoEVfvQKBgQC/PQS4vT6XORRbHYzq42wPYPLYFAtWZzg2\nqc8iTDEfkZ6EusXrlPxnTmwHNSAuMebyoE9ziAVCcssjsyBtkjjnKc1NE08y8284\n3Cs7JFfPhhG/ghOtv2R6mW9Z1UtqfGWQTqMufm7BBFw4vZNWljjjITFvMdvenSMb\nXYwBu/iXFwKBgFLEW3IUJuCFoF9vI32VxVj+UvOd1RKLO4Q2ypxbW32S9cgBWPab\nOWFaOGL662QRAtCl+zfneYiQiIpEEjvkgdbMe9bRK8dt3H682jXQiXtIPgQFoaNy\nBIDB4oRsh9IAOqaqyqeoSHzMu6Pl+C4YNv81dfTMtSOg5KzF4wBsJIwVAoGAT9uE\nKDzmcTGlvXK2kLONQVLDtdWQ8nDB+ZmpZHIapUsivdxcn8akK+OEmvHlUUUHYtPs\nuZrYT2ouR+caKIdB+c3r7D6e+PDMxhqydszzWjZrHOSNoSVmKQf/hqzaBEqUAtHD\ntLuZNkLC2/LWHvc2JCqNQRi57tkBewDyYRsEcNsCgYA9j2Ywv15ef1xBGy0dNqkC\nM39PjCf4XKOjTcyi0UUMQ9LR34WQnYpKULcbz+B9mOTgZ2BjK2tvXdMF+CayjjPo\nMsf4Iru8NbbP/KvpmjIyz6tVjdt4MKs92zueLgQ5IyoAY/jA/mICTnWSjpOOnbtp\n7lRlHMZMLIC0G8es9wujbQ==\n-----END PRIVATE KEY-----\n"
)
