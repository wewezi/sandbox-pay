package routes

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestTokenAdministrationDefaultsAndOneTimeDisclosure(t *testing.T) {
	h := newHarness(t, nil)
	adminPost := func(path string, body map[string]any) *resp {
		t.Helper()
		return h.do(t, reqSpec{Method: "POST", Path: path, Body: body,
			Headers: withHeaders(h.auth, map[string]string{"Content-Type": "application/json"})})
	}
	created := adminPost("/v1/api/admin/api-keys", map[string]any{"name": "本地开发"})
	eq(t, "create token without a new project", created.Status, http.StatusOK)
	issued := created.data()
	eq(t, "default project", int64(num(t, issued["groupId"], "group id")), h.groupID)
	secret := str(t, issued["key"], "issued token")
	if !strings.HasPrefix(secret, "sp_sandbox_") {
		t.Fatal("creation must disclose a usable API token")
	}
	id := int64(num(t, issued["id"], "token id"))
	listed := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/api-keys", Headers: h.auth})
	eq(t, "list tokens", listed.Status, http.StatusOK)
	eq(t, "default token and new token", len(listed.dataArr()), 2)
	lacksSubstr(t, listed.Raw, secret, "token list")
	lacksSubstr(t, listed.Raw, testAPIKey, "seed token list")
	for _, item := range listed.dataArr() {
		metadata := item.(map[string]any)
		if _, exposed := metadata["key"]; exposed {
			t.Fatal("token list must not expose full credentials")
		}
		if str(t, metadata["groupName"], "group name") == "" || str(t, metadata["keyPrefix"], "token prefix") == "" {
			t.Fatal("token list is missing project/display metadata")
		}
	}
	stored, err := h.store.ApiKeys.FindByKey(secret)
	if err != nil || stored == nil || stored.ID != id || stored.Key != "" || stored.KeyHash == secret {
		t.Fatalf("token must persist as a hash and remain discoverable: %v", err)
	}
	off := adminPost(fmt.Sprintf("/v1/api/admin/api-keys/%d/toggle", id), nil)
	eq(t, "disable token", off.Status, http.StatusOK)
	if _, exposed := off.data()["key"]; exposed {
		t.Fatal("toggle receipt must not re-disclose credential")
	}
	project := adminPost("/v1/api/admin/projects", map[string]any{"name": "另一个项目"})
	eq(t, "create project without issuing tokens", project.Status, http.StatusOK)
	projectID := int64(num(t, project.data()["id"], "project id"))
	eq(t, "project has no implicit token", len(project.data()["keys"].([]any)), 0)
	projectToken := adminPost("/v1/api/admin/api-keys", map[string]any{"name": "CI", "groupId": projectID})
	eq(t, "create token in selected project", projectToken.Status, http.StatusOK)
	eq(t, "selected project id", int64(num(t, projectToken.data()["groupId"], "group id")), projectID)
	eq(t, "selected project label", projectToken.data()["groupName"], "另一个项目")
	filtered := h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/api-keys?groupId=%d", projectID), Headers: h.auth})
	eq(t, "filter project tokens", filtered.Status, http.StatusOK)
	eq(t, "one selected project token", len(filtered.dataArr()), 1)
	configuration := h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/groups/%d/webhook-secret", projectID), Headers: h.auth})
	eq(t, "explicit webhook secret request", configuration.Status, http.StatusOK)
	webhookSecret := str(t, configuration.data()["secret"], "webhook secret")
	if !strings.HasPrefix(webhookSecret, "whsec_") || webhookSecret == projectToken.data()["key"] {
		t.Fatal("webhook must use a separate signing credential")
	}
	groups := h.do(t, reqSpec{Method: "GET", Path: "/v1/api/admin/groups", Headers: h.auth})
	lacksSubstr(t, groups.Raw, webhookSecret, "project list")
	unauthorized := h.do(t, reqSpec{Method: "GET", Path: fmt.Sprintf("/v1/api/admin/groups/%d/webhook-secret", projectID)})
	eq(t, "webhook configuration requires administrator", unauthorized.Status, http.StatusUnauthorized)
	eq(t, "disable default project", adminPost(fmt.Sprintf("/v1/api/admin/groups/%d/toggle", h.groupID), nil).Status, http.StatusOK)
	rejected := adminPost("/v1/api/admin/api-keys", map[string]any{"name": "default disabled"})
	eq(t, "default creation cannot reactivate disabled project", rejected.Status, http.StatusBadRequest)
	defaultProject, err := h.store.Groups.FindByID(h.groupID)
	if err != nil || defaultProject == nil || defaultProject.IsActive {
		t.Fatalf("creation reactivated project: %v", err)
	}
}
