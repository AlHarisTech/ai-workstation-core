package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type GitHubBridge struct {
	token   string
	owner   string
	repo    string
	client  *http.Client
}

func NewGitHubBridge(token, owner, repo string) *GitHubBridge {
	return &GitHubBridge{
		token:  token,
		owner:  owner,
		repo:   repo,
		client: &http.Client{},
	}
}

func (gb *GitHubBridge) do(method, path string, body io.Reader) (*http.Response, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s%s", gb.owner, gb.repo, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ai-workstation-mro/1.0")
	req.Header.Set("Authorization", "Bearer "+gb.token)
	return gb.client.Do(req)
}

func (gb *GitHubBridge) ValidateAccess() error {
	if gb.token == "" {
		return fmt.Errorf("GITHUB_TOKEN not set")
	}
	resp, err := gb.do("GET", "", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("token authentication failed (401)")
	}
	if resp.StatusCode == 404 {
		return fmt.Errorf("repository not found (404)")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("access check failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

type createTagRequest struct {
	Tag     string `json:"tag"`
	Message string `json:"message"`
	Object  string `json:"object"`
	Type    string `json:"type"`
}

type createTagResponse struct {
	SHA string `json:"sha"`
}

func (gb *GitHubBridge) CreateTag(tagName, commitSHA string) (string, error) {
	body := createTagRequest{
		Tag:     tagName,
		Message: fmt.Sprintf("Release %s", tagName),
		Object:  commitSHA,
		Type:    "commit",
	}
	raw, _ := json.Marshal(body)
	resp, err := gb.do("POST", "/git/tags", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		rBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create tag API error %d: %s", resp.StatusCode, string(rBody))
	}
	var result createTagResponse
	json.NewDecoder(resp.Body).Decode(&result)

	refBody := fmt.Sprintf(`{"ref":"refs/tags/%s","sha":"%s"}`, tagName, result.SHA)
	refResp, err := gb.do("POST", "/git/refs", strings.NewReader(refBody))
	if err != nil {
		return "", err
	}
	defer refResp.Body.Close()
	if refResp.StatusCode >= 400 {
		rBody, _ := io.ReadAll(refResp.Body)
		return "", fmt.Errorf("create tag ref API error %d: %s", refResp.StatusCode, string(rBody))
	}

	return result.SHA, nil
}

func (gb *GitHubBridge) DeleteTag(tagName, tagSHA string) error {
	resp, err := gb.do("DELETE", fmt.Sprintf("/git/refs/tags/%s", tagName), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type createReleaseRequest struct {
	TagName         string `json:"tag_name"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	TargetCommitish string `json:"target_commitish"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
}

type createReleaseResponse struct {
	HTMLURL string `json:"html_url"`
}

func (gb *GitHubBridge) CreateRelease(tagName, commitSHA, releaseNotes string) (string, error) {
	body := createReleaseRequest{
		TagName:         tagName,
		Name:            fmt.Sprintf("Release %s", tagName),
		Body:            releaseNotes,
		TargetCommitish: commitSHA,
		Draft:           false,
		Prerelease:      false,
	}
	raw, _ := json.Marshal(body)
	resp, err := gb.do("POST", "/releases", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		rBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create release API error %d: %s", resp.StatusCode, string(rBody))
	}
	var result createReleaseResponse
	json.NewDecoder(resp.Body).Decode(&result)
	return result.HTMLURL, nil
}
