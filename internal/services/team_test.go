package services

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
	"sandbox-pay/internal/config"
	"sandbox-pay/internal/db"
	"sandbox-pay/internal/httperr"
	"sandbox-pay/internal/jwt"
)

func teamFixture(t *testing.T) (*db.Store, *AuthService, *TeamService, *db.User, config.Map) {
	t.Helper()
	store := newTestStore(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("root-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Users.Insert("root-admin", string(hash), db.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	root, err := store.Users.FindByUsername("root-admin")
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Map{"JWT_SECRET": "team-test-secret-0123456789abcdef-feda9876543210", "JWT_EXPIRES_IN": "60m"}
	return store, NewAuthService(store, cfg), NewTeamService(store), root, cfg
}

func teamStatus(t *testing.T, err error, status int) {
	t.Helper()
	if e := httperr.As(err); e == nil || e.Status != status {
		t.Fatalf("expected status %d, got %v", status, err)
	}
}

func TestTeamSessionRevocationAndLegacyJWT(t *testing.T) {
	store, auth, _, root, cfg := teamFixture(t)
	first, err := auth.IssueLocalToken(root, SessionMetadata{ClientIP: "10.0.0.1", UserAgent: "device one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := auth.IssueLocalToken(root, SessionMetadata{ClientIP: "10.0.0.2", UserAgent: "device two"})
	if err != nil {
		t.Fatal(err)
	}
	_, currentID, err := auth.ValidateSession(jwt.Verify(first.Token, cfg.Get("JWT_SECRET")))
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.Sessions(root.ID, currentID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions=%d", len(sessions))
	}
	var current bool
	for _, session := range sessions {
		if session.Current {
			current = session.ID == currentID && session.ClientIP == "10.0.0.1" && session.UserAgent == "device one"
		}
	}
	if !current {
		t.Fatal("current session metadata missing")
	}
	if err := store.Users.RevokeOtherSessions(root.ID, currentID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.ValidateSession(jwt.Verify(first.Token, cfg.Get("JWT_SECRET"))); err != nil {
		t.Fatal("current session revoked", err)
	}
	_, _, err = auth.ValidateSession(jwt.Verify(second.Token, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
	if err := auth.RevokeToken(first.Token); err != nil {
		t.Fatal(err)
	}
	_, _, err = auth.ValidateSession(jwt.Verify(first.Token, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
	legacy := jwt.Sign(map[string]any{"sub": root.ID, "username": root.Username, "role": root.Role}, cfg.Get("JWT_SECRET"), 60)
	_, _, err = auth.ValidateSession(jwt.Verify(legacy, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
}

func TestTeamPoliciesAndAtomicProjectMembership(t *testing.T) {
	store, auth, team, root, cfg := teamFixture(t)
	adminRole, inactive := db.RoleAdmin, false
	teamStatus(t, team.SetAccess(root.ID, root.ID, nil, &inactive), 409)
	teamStatus(t, team.SetAccess(root.ID, root.ID, &adminRole, nil), 409)
	created, err := team.CreateUser(root.ID, "project-admin", "member-password", db.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if created.IsActive != true || len(created.GroupIDs) != 0 {
		t.Fatal("new admin must start active and without project access")
	}
	_, err = team.CreateUser(created.ID, "unauthorized", "member-password", db.RoleSuperAdmin)
	teamStatus(t, err, 403)
	project, err := store.Groups.Insert("team-project", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := team.SetProjects(root.ID, created.ID, []int64{project.ID}); err != nil {
		t.Fatal(err)
	}
	teamStatus(t, team.SetProjects(root.ID, created.ID, []int64{project.ID, project.ID}), 400)
	ids, err := store.Users.ProjectIDs(created.ID)
	if err != nil || len(ids) != 1 || ids[0] != project.ID {
		t.Fatalf("membership not atomic: %v %v", ids, err)
	}
	user, err := auth.ValidateUser("project-admin", "member-password")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.IssueLocalToken(user)
	if err != nil {
		t.Fatal(err)
	}
	if err := team.SetAccess(root.ID, created.ID, nil, &inactive); err != nil {
		t.Fatal(err)
	}
	_, _, err = auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
	_, err = auth.ValidateUser("project-admin", "member-password")
	teamStatus(t, err, 401)
	active := true
	if err := team.SetAccess(root.ID, created.ID, nil, &active); err != nil {
		t.Fatal(err)
	}
	if _, err = auth.ValidateUser("project-admin", "member-password"); err != nil {
		t.Fatal(err)
	}
}

func TestTeamPasswordChangeAndCLIResetInvalidateSessions(t *testing.T) {
	store, auth, team, root, cfg := teamFixture(t)
	token, err := auth.IssueLocalToken(root)
	if err != nil {
		t.Fatal(err)
	}
	teamStatus(t, auth.ChangePassword(root.ID, "wrong-password", "new-password"), 400)
	if _, _, err := auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET"))); err != nil {
		t.Fatal("bad old password invalidated session")
	}
	if err := auth.ChangePassword(root.ID, "root-password", "new-password"); err != nil {
		t.Fatal(err)
	}
	_, _, err = auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
	root, err = auth.ValidateUser("root-admin", "new-password")
	if err != nil {
		t.Fatal(err)
	}
	token, err = auth.IssueLocalToken(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := team.ResetPassword(root.ID, root.ID, "reset-password"); err != nil {
		t.Fatal(err)
	}
	_, _, err = auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
	root, err = auth.ValidateUser("root-admin", "reset-password")
	if err != nil {
		t.Fatal(err)
	}
	token, err = auth.IssueLocalToken(root)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("cli-password"), bcrypt.MinCost)
	if err := store.Users.UpdateCredentials("root-admin", string(hash), db.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	_, _, err = auth.ValidateSession(jwt.Verify(token.Token, cfg.Get("JWT_SECRET")))
	teamStatus(t, err, 401)
	if _, err := auth.IssueLocalToken(root); err == nil {
		t.Fatal("stale password validated user was allowed to create a session")
	}
}
