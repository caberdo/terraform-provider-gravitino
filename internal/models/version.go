package models

type VersionResponse struct {
	Code    int     `json:"code"`
	Version Version `json:"version"`
}

type Version struct {
	Version     string `json:"version"`
	CompileDate string `json:"compileDate"`
	GitCommit   string `json:"gitCommit"`
}
