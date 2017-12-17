package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

const maxTry = 5

type mapping struct {
	Sequences []sequence `json:"sequences"`
}

type sequence struct {
	Clips []clip `json:"clips"`
}

type clip struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

func getMapHandler(c Config, client *storage.Client) http.HandlerFunc {
	bucketHandle := client.Bucket(c.BucketName)
	logger := c.logger()
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		prefix := strings.TrimLeft(r.URL.Path, "/")
		if prefix == "" {
			http.Error(w, "prefix cannot be empty", http.StatusBadRequest)
			return
		}
		m, err := getPrefixMapping(prefix, c, bucketHandle)
		if err != nil && err != iterator.Done {
			logger.WithError(err).WithField("prefix", prefix).Error("failed to map request")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m = appendExtraResources(r, c, m)
		m, err = signedURLs(c, m)
		if err != nil {
			logger.WithError(err).Error("failed to sign URLs")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	}
}

func appendExtraResources(r *http.Request, config Config, m mapping) mapping {
	resources := r.URL.Query().Get(config.ExtraResourcesToken)
	for _, resource := range strings.Split(resources, ",") {
		if resource != "" {
			m.Sequences = append(m.Sequences, sequence{
				Clips: []clip{{Type: "source", Path: resource}},
			})
		}
	}
	return m
}

func getPrefixMapping(prefix string, config Config, bucketHandle *storage.BucketHandle) (mapping, error) {
	m := mapping{Sequences: []sequence{}}
	for _, p := range getPrefixes(prefix, config) {
		sequences, err := expandPrefix(p, config, bucketHandle)
		if err != nil {
			return m, err
		}
		m.Sequences = append(m.Sequences, sequences...)
	}
	return m, nil
}

func getPrefixes(originalPrefix string, config Config) []string {
	prefixes := []string{originalPrefix}
	_, lastPart := path.Split(originalPrefix)
	for _, p := range config.MapExtraPrefixes {
		prefixes = append(prefixes, path.Join(p, lastPart))
	}
	return prefixes
}

func expandPrefix(prefix string, config Config, bucketHandle *storage.BucketHandle) ([]sequence, error) {
	var err error
	var filterRegex string
	if strings.Contains(prefix, "__HD") {
		filterRegex = config.MapRegexHDFilter
		prefix = strings.Replace(prefix, "__HD", "", 1)
	} else {
		filterRegex = config.MapRegexFilter
	}
	for i := 0; i < maxTry; i++ {
		iter := bucketHandle.Objects(context.Background(), &storage.Query{
			Prefix:    prefix,
			Delimiter: "/",
		})
		var obj *storage.ObjectAttrs
		sequences := []sequence{}
		obj, err = iter.Next()
		for ; err == nil; obj, err = iter.Next() {
			filename := filepath.Base(obj.Name)
			matched, _ := regexp.MatchString(filterRegex, filename)
			if matched {
				sequences = append(sequences, sequence{
					Clips: []clip{{Type: "source", Path: "/" + obj.Bucket + "/" + obj.Name}},
				})
			}
		}
		if err == iterator.Done {
			return sequences, nil
		}
	}
	return nil, err
}

func signedURLs(config Config, m mapping) (mapping, error) {
	opts, err := config.SignConfig.Options()
	if err != nil || opts == nil {
		return m, err
	}
	seqs := m.Sequences
	for s, seq := range seqs {
		for c, clip := range seq.Clips {
			path, err := signedPath(clip.Path, opts)
			if err != nil {
				return m, err
			}
			clip.Path = path
			seq.Clips[c] = clip
		}
		seqs[s] = seq
	}
	m.Sequences = seqs
	return m, nil
}

func signedPath(path string, opts *storage.SignedURLOptions) (string, error) {
	parts := strings.SplitN(path, "/", 3)
	if len(parts) != 3 {
		return path, nil
	}
	bucketName := parts[1]
	objectKey := parts[2]
	rawSignedURL, err := storage.SignedURL(bucketName, objectKey, opts)
	if err != nil {
		return "", err
	}
	signedURL, err := url.Parse(rawSignedURL)
	if err != nil {
		return "", err
	}
	return signedURL.RequestURI(), nil
}
