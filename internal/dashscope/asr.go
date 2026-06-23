package dashscope

import (
	"encoding/base64"
	"fmt"
)

const (
	ModelASRFlash     = "qwen3-asr-flash"
	ModelASRFiletrans = "qwen3-asr-flash-filetrans"
	multimodalGenPath = "/services/aigc/multimodal-generation/generation"
)

// ASRResult holds the transcription output
type ASRResult struct {
	Text string
}

// Transcribe sends audio to Qwen3-ASR via the multimodal generation endpoint.
// wavData should be WAV file bytes. context is optional text context for better
// recognition. itn toggles inverse text normalization (spoken numbers -> digits).
func (c *Client) Transcribe(wavData []byte, context string, itn bool) (*ASRResult, error) {
	audioURI := "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(wavData)

	systemText := ""
	if context != "" {
		systemText = context
	}

	body := map[string]any{
		"model": ModelASRFlash,
		"input": map[string]any{
			"messages": []map[string]any{
				{
					"role": "system",
					"content": []map[string]string{
						{"text": systemText},
					},
				},
				{
					"role": "user",
					"content": []map[string]string{
						{"audio": audioURI},
					},
				},
			},
		},
		"parameters": map[string]any{
			"asr_options": map[string]any{
				"enable_itn": itn,
			},
		},
	}

	resp, err := c.post(multimodalGenPath, body)
	if err != nil {
		return nil, err
	}

	// Parse: output.choices[0].message.content[0].text
	output, ok := resp["output"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response: missing output")
	}

	choices, ok := output["choices"].([]any)
	if !ok || len(choices) == 0 {
		return nil, fmt.Errorf("unexpected response: missing choices")
	}

	choice, ok := choices[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response: invalid choice")
	}

	message, ok := choice["message"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response: missing message")
	}

	content, ok := message["content"].([]any)
	if !ok || len(content) == 0 {
		return nil, fmt.Errorf("unexpected response: missing content")
	}

	item, ok := content[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected response: invalid content item")
	}

	text, ok := item["text"].(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response: missing text")
	}

	return &ASRResult{Text: text}, nil
}
