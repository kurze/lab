module github.com/kurze/lab/scrutineer

go 1.26.3

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/google/go-github/v72 v72.0.0
	github.com/kurze/lab/agentcore v0.0.0
	gitlab.com/gitlab-org/api/client-go v1.46.0
	golang.org/x/text v0.32.0
)

require (
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.8 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/time v0.14.0 // indirect
)

replace github.com/kurze/lab/agentcore => ../agentcore
