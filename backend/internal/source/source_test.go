package source

import "testing"

// Both helpers below decide which host and path the server will call on behalf
// of a user, so they are the SSRF boundary for Azure DevOps and GitHub.

func TestNormalizeAzureOrg(t *testing.T) {
	accepted := map[string]string{
		"contoso":                          "contoso",
		"  contoso  ":                      "contoso",
		"https://dev.azure.com/contoso":    "contoso",
		"https://dev.azure.com/contoso/":   "contoso",
		"https://contoso.visualstudio.com": "contoso",
		"my-org_1.2":                       "my-org_1.2",
	}
	for in, want := range accepted {
		got, err := NormalizeAzureOrg(in)
		if err != nil {
			t.Errorf("NormalizeAzureOrg(%q) unexpected error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeAzureOrg(%q) = %q, want %q", in, got, want)
		}
	}

	rejected := []string{
		"",
		"   ",
		"contoso/project",
		"http://dev.azure.com/contoso",
		"https://dev.azure.com/",
		"https://evil.example.com/contoso",
		"https://dev.azure.com.evil.com/contoso",
		"https://user:pass@dev.azure.com/contoso",
		"-contoso",
		"cont oso",
		"contoso?x=1",
	}
	for _, in := range rejected {
		if got, err := NormalizeAzureOrg(in); err == nil {
			t.Errorf("NormalizeAzureOrg(%q) = %q, want an error", in, got)
		}
	}
}

func TestSplitRepo(t *testing.T) {
	owner, name, err := splitRepo("https://github.com/jatin-bhatia1/estimeet")
	if err != nil || owner != "jatin-bhatia1" || name != "estimeet" {
		t.Fatalf("splitRepo(url) = %q, %q, %v", owner, name, err)
	}
	if owner, name, err = splitRepo(" octocat/Hello-World "); err != nil || owner != "octocat" || name != "Hello-World" {
		t.Fatalf("splitRepo(slug) = %q, %q, %v", owner, name, err)
	}

	for _, in := range []string{"", "octocat", "octocat/", "/hello", "a/b/c", "octo cat/hello", "../../etc", "octocat/hello?x"} {
		if o, n, err := splitRepo(in); err == nil {
			t.Errorf("splitRepo(%q) = %q, %q, want an error", in, o, n)
		}
	}
}
