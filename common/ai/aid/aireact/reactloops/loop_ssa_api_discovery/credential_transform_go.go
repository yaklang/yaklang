package loop_ssa_api_discovery

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type credentialTransformResult struct {
	Algorithm string `json:"algorithm"`
	Input     string `json:"input"`
	Output    string `json:"output"`
	UsageHint string `json:"usage_hint,omitempty"`
}

func transformCredentialGoParams(algorithm, input, salt, saltPosition, key, outputFormat string, uppercase bool) (*credentialTransformResult, error) {
	algorithm = strings.ToLower(strings.TrimSpace(algorithm))
	if input == "" {
		return nil, utils.Error("input required")
	}
	data := applySalt(input, salt, saltPosition)
	var out []byte
	switch algorithm {
	case "md5":
		sum := md5.Sum([]byte(data))
		out = sum[:]
	case "sha1":
		sum := sha1.Sum([]byte(data))
		out = sum[:]
	case "sha256":
		sum := sha256.Sum256([]byte(data))
		out = sum[:]
	case "sha512":
		sum := sha512.Sum512([]byte(data))
		out = sum[:]
	case "base64":
		return &credentialTransformResult{
			Algorithm: algorithm, Input: input,
			Output:    base64.StdEncoding.EncodeToString([]byte(data)),
			UsageHint: "use output as field value",
		}, nil
	case "base64url":
		return &credentialTransformResult{
			Algorithm: algorithm, Input: input,
			Output:    base64.RawURLEncoding.EncodeToString([]byte(data)),
			UsageHint: "use output as field value",
		}, nil
	case "url":
		return &credentialTransformResult{
			Algorithm: algorithm, Input: input,
			Output:    url.QueryEscape(data),
			UsageHint: "use output as field value",
		}, nil
	case "hex":
		out = []byte(data)
	case "hmac-md5":
		if strings.TrimSpace(key) == "" {
			return nil, utils.Error("hmac requires key")
		}
		m := hmac.New(md5.New, []byte(key))
		m.Write([]byte(data))
		out = m.Sum(nil)
	case "hmac-sha1":
		if strings.TrimSpace(key) == "" {
			return nil, utils.Error("hmac requires key")
		}
		m := hmac.New(sha1.New, []byte(key))
		m.Write([]byte(data))
		out = m.Sum(nil)
	case "hmac-sha256":
		if strings.TrimSpace(key) == "" {
			return nil, utils.Error("hmac requires key")
		}
		m := hmac.New(sha256.New, []byte(key))
		m.Write([]byte(data))
		out = m.Sum(nil)
	case "hmac-sha512":
		if strings.TrimSpace(key) == "" {
			return nil, utils.Error("hmac requires key")
		}
		m := hmac.New(sha512.New, []byte(key))
		m.Write([]byte(data))
		out = m.Sum(nil)
	default:
		return nil, utils.Errorf("unsupported algorithm %q", algorithm)
	}
	formatted := formatCredentialOutput(out, outputFormat, uppercase, algorithm)
	return &credentialTransformResult{
		Algorithm: algorithm, Input: input, Output: formatted,
		UsageHint: "use output in login POST password (or named) field",
	}, nil
}

func applySalt(input, salt, position string) string {
	salt = strings.TrimSpace(salt)
	if salt == "" {
		return input
	}
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "prefix":
		return salt + input
	case "suffix", "":
		return input + salt
	default:
		return input + salt
	}
}

func formatCredentialOutput(out []byte, outputFormat string, uppercase bool, algorithm string) string {
	outputFormat = strings.ToLower(strings.TrimSpace(outputFormat))
	if outputFormat == "" {
		if algorithm == "hex" || strings.HasPrefix(algorithm, "sha") || algorithm == "md5" || strings.HasPrefix(algorithm, "hmac-") {
			outputFormat = "hex"
		}
	}
	switch outputFormat {
	case "base64":
		return base64.StdEncoding.EncodeToString(out)
	case "lower":
		return strings.ToLower(string(out))
	case "upper":
		return strings.ToUpper(string(out))
	default:
		s := hex.EncodeToString(out)
		if uppercase {
			return strings.ToUpper(s)
		}
		return s
	}
}

func credentialTransformJSON(algorithm, input, salt, saltPosition, key, outputFormat string, uppercase bool) (string, error) {
	res, err := transformCredentialGoParams(algorithm, input, salt, saltPosition, key, outputFormat, uppercase)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(res, "", "  ")
	return string(b), err
}
