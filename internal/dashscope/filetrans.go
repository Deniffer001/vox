package dashscope

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const transcriptionPath = "/services/audio/asr/transcription"

// TranscribeURL transcribes a publicly accessible audio URL via the async
// filetrans model (qwen3-asr-flash-filetrans, up to 12h / 2GB).
func (c *Client) TranscribeURL(fileURL string, itn bool) (*ASRResult, error) {
	return c.transcribeAsync(fileURL, false, itn)
}

// TranscribeLocalFile uploads a local file to DashScope's temporary store and
// transcribes it via the async filetrans model. No external object storage is
// required — the file is referenced as oss:// and resolved server-side.
func (c *Client) TranscribeLocalFile(localPath string, itn bool) (*ASRResult, error) {
	ossURL, err := c.uploadInstant(localPath, ModelASRFiletrans)
	if err != nil {
		return nil, fmt.Errorf("upload: %w", err)
	}
	return c.transcribeAsync(ossURL, true, itn)
}

func (c *Client) transcribeAsync(fileURL string, resolveOSS, itn bool) (*ASRResult, error) {
	taskID, err := c.submitTranscription(fileURL, resolveOSS, itn)
	if err != nil {
		return nil, err
	}
	transcriptURL, err := c.pollTranscription(taskID)
	if err != nil {
		return nil, err
	}
	return c.downloadTranscript(transcriptURL)
}

// --- temporary upload (getPolicy + OSS PostObject) ---

type uploadPolicy struct {
	Policy       string `json:"policy"`
	Signature    string `json:"signature"`
	UploadDir    string `json:"upload_dir"`
	UploadHost   string `json:"upload_host"`
	OSSAccessKey string `json:"oss_access_key_id"`
	ObjectACL    string `json:"x_oss_object_acl"`
	ForbidWrite  string `json:"x_oss_forbid_overwrite"`
}

func (c *Client) getUploadPolicy(model string) (*uploadPolicy, error) {
	req, err := http.NewRequest("GET", httpEndpoint+"/uploads?action=getPolicy&model="+model, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getPolicy HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		Data uploadPolicy `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if out.Data.UploadHost == "" {
		return nil, fmt.Errorf("getPolicy: empty policy: %s", string(body))
	}
	return &out.Data, nil
}

func (c *Client) uploadInstant(localPath, model string) (string, error) {
	pol, err := c.getUploadPolicy(model)
	if err != nil {
		return "", err
	}

	f, err := os.Open(localPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	key := pol.UploadDir + "/" + filepath.Base(localPath)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	// OSS PostObject policy fields — must precede the file part.
	for _, field := range [][2]string{
		{"key", key},
		{"policy", pol.Policy},
		{"OSSAccessKeyId", pol.OSSAccessKey},
		{"signature", pol.Signature},
		{"success_action_status", "200"},
		{"x-oss-object-acl", pol.ObjectACL},
		{"x-oss-forbid-overwrite", pol.ForbidWrite},
	} {
		if err := w.WriteField(field[0], field[1]); err != nil {
			return "", err
		}
	}
	fw, err := w.CreateFormFile("file", filepath.Base(localPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, f); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", pol.UploadHost, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return "", fmt.Errorf("oss upload HTTP %d: %s", resp.StatusCode, string(body))
	}
	return "oss://" + key, nil
}

// --- async transcription task ---

func (c *Client) submitTranscription(fileURL string, resolveOSS, itn bool) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":      ModelASRFiletrans,
		"input":      map[string]any{"file_url": fileURL},
		"parameters": map[string]any{"channel_id": []int{0}, "enable_itn": itn},
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", httpEndpoint+transcriptionPath, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
	if resolveOSS {
		req.Header.Set("X-DashScope-OssResourceResolve", "enable")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("submit HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		Output struct {
			TaskID string `json:"task_id"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", err
	}
	if out.Output.TaskID == "" {
		return "", fmt.Errorf("no task_id in response: %s", string(respBody))
	}
	return out.Output.TaskID, nil
}

func (c *Client) pollTranscription(taskID string) (string, error) {
	// Poll up to ~20 minutes (enough headroom for multi-hour audio jobs).
	for i := 0; i < 600; i++ {
		req, err := http.NewRequest("GET", httpEndpoint+"/tasks/"+taskID, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", err
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var out struct {
			Output struct {
				TaskStatus string `json:"task_status"`
				Message    string `json:"message"`
				Result     struct {
					TranscriptionURL string `json:"transcription_url"`
				} `json:"result"`
			} `json:"output"`
		}
		if err := json.Unmarshal(respBody, &out); err != nil {
			return "", err
		}

		switch out.Output.TaskStatus {
		case "SUCCEEDED":
			if out.Output.Result.TranscriptionURL == "" {
				return "", fmt.Errorf("task succeeded but no transcription_url: %s", string(respBody))
			}
			return out.Output.Result.TranscriptionURL, nil
		case "FAILED", "CANCELED":
			return "", fmt.Errorf("task %s: %s", out.Output.TaskStatus, out.Output.Message)
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("transcription timed out")
}

func (c *Client) downloadTranscript(url string) (*ASRResult, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download transcript HTTP %d", resp.StatusCode)
	}

	var tr struct {
		Transcripts []struct {
			Text string `json:"text"`
		} `json:"transcripts"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, err
	}

	var parts []string
	for _, t := range tr.Transcripts {
		if t.Text != "" {
			parts = append(parts, t.Text)
		}
	}
	return &ASRResult{Text: strings.Join(parts, "\n")}, nil
}
