package service

import "testing"

func TestShouldCaptureRequestAuditScopeSemantics(t *testing.T) {
	groupID := int64(20)
	otherGroupID := int64(30)

	tests := []struct {
		name          string
		userID        int64
		groupID       *int64
		scopeUserIDs  []int64
		scopeGroupIDs []int64
		want          bool
	}{
		{
			name:    "empty scopes audit all requests",
			userID:  10,
			groupID: &groupID,
			want:    true,
		},
		{
			name:         "only users audits matching user across all groups",
			userID:       10,
			groupID:      &groupID,
			scopeUserIDs: []int64{10},
			want:         true,
		},
		{
			name:         "only users skips non matching user",
			userID:       11,
			groupID:      &groupID,
			scopeUserIDs: []int64{10},
			want:         false,
		},
		{
			name:          "only groups audits all users in matching group",
			userID:        11,
			groupID:       &groupID,
			scopeGroupIDs: []int64{20},
			want:          true,
		},
		{
			name:          "only groups skips non matching group",
			userID:        11,
			groupID:       &otherGroupID,
			scopeGroupIDs: []int64{20},
			want:          false,
		},
		{
			name:          "users and groups require intersection",
			userID:        10,
			groupID:       &groupID,
			scopeUserIDs:  []int64{10},
			scopeGroupIDs: []int64{20},
			want:          true,
		},
		{
			name:          "users and groups skip matching user outside group",
			userID:        10,
			groupID:       &otherGroupID,
			scopeUserIDs:  []int64{10},
			scopeGroupIDs: []int64{20},
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldCaptureRequestAudit(tt.userID, tt.groupID, tt.scopeUserIDs, tt.scopeGroupIDs)
			if got != tt.want {
				t.Fatalf("ShouldCaptureRequestAudit() = %v, want %v", got, tt.want)
			}
		})
	}
}
