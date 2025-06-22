package asana

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const BaseURL = "https://app.asana.com/api/1.0"

type Client struct {
	APIKey     string
	HTTPClient *http.Client
}

type ListProjectsArgs struct {
	WorkspaceGID string `json:"workspace_gid,omitempty" jsonschema_description:"The workspace GID to list projects from (optional - will use default workspace if only one exists)"`
}

type ListProjectTasksArgs struct {
	ProjectGID string `json:"project_gid" jsonschema_description:"The project GID to list tasks from"`
}

type ListUserTasksArgs struct {
	AssigneeGID  string `json:"assignee_gid" jsonschema_description:"The user GID to get assigned tasks for"`
	WorkspaceGID string `json:"workspace_gid,omitempty" jsonschema_description:"The workspace GID to search within (optional - will use default workspace if only one exists)"`
}

type Project struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type Workspace struct {
	GID  string `json:"gid"`
	Name string `json:"name"`
}

type Task struct {
	GID       string `json:"gid"`
	Name      string `json:"name"`
	Completed bool   `json:"completed"`
	Notes     string `json:"notes"`
}

type ListResponse struct {
	Data []json.RawMessage `json:"data"`
}

func NewClient(apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		APIKey:     apiKey,
		HTTPClient: httpClient,
	}
}

func (c *Client) makeRequest(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) GetWorkspaces() ([]Workspace, error) {
	body, err := c.makeRequest("GET", "/workspaces")
	if err != nil {
		return nil, err
	}

	var response ListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var workspaces []Workspace
	for _, item := range response.Data {
		var workspace Workspace
		if err := json.Unmarshal(item, &workspace); err != nil {
			continue // Skip malformed entries
		}
		workspaces = append(workspaces, workspace)
	}

	return workspaces, nil
}

func (c *Client) getDefaultWorkspace() (string, error) {
	workspaces, err := c.GetWorkspaces()
	if err != nil {
		return "", fmt.Errorf("failed to get workspaces: %w", err)
	}

	if len(workspaces) == 0 {
		return "", fmt.Errorf("no workspaces found")
	}

	if len(workspaces) == 1 {
		return workspaces[0].GID, nil
	}

	return "", fmt.Errorf("multiple workspaces found (%d), workspace_gid must be specified", len(workspaces))
}

func (c *Client) ListProjects(workspaceGID string) ([]Project, error) {
	// Use default workspace if not specified
	if workspaceGID == "" {
		defaultWorkspace, err := c.getDefaultWorkspace()
		if err != nil {
			return nil, err
		}
		workspaceGID = defaultWorkspace
	}

	path := fmt.Sprintf("/workspaces/%s/projects", workspaceGID)
	body, err := c.makeRequest("GET", path)
	if err != nil {
		return nil, err
	}

	var response ListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var projects []Project
	for _, item := range response.Data {
		var project Project
		if err := json.Unmarshal(item, &project); err != nil {
			continue // Skip malformed entries
		}
		projects = append(projects, project)
	}

	return projects, nil
}

func (c *Client) ListProjectTasks(projectGID string) ([]Task, error) {
	path := fmt.Sprintf("/projects/%s/tasks?completed_since=now", projectGID)
	body, err := c.makeRequest("GET", path)
	if err != nil {
		return nil, err
	}

	var response ListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var tasks []Task
	for _, item := range response.Data {
		var task Task
		if err := json.Unmarshal(item, &task); err != nil {
			continue // Skip malformed entries
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

func (c *Client) ListUserTasks(assigneeGID, workspaceGID string) ([]Task, error) {
	// Use default workspace if not specified
	if workspaceGID == "" {
		defaultWorkspace, err := c.getDefaultWorkspace()
		if err != nil {
			return nil, err
		}
		workspaceGID = defaultWorkspace
	}

	path := fmt.Sprintf("/tasks?assignee=%s&workspace=%s&completed_since=now", assigneeGID, workspaceGID)
	body, err := c.makeRequest("GET", path)
	if err != nil {
		return nil, err
	}

	var response ListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var tasks []Task
	for _, item := range response.Data {
		var task Task
		if err := json.Unmarshal(item, &task); err != nil {
			continue // Skip malformed entries
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}