package domain

import "testing"

func a(status AssignmentStatus) TaskAssignment { return TaskAssignment{Status: status} }

func TestDeriveTaskStatus(t *testing.T) {
	cases := []struct {
		name string
		in   []TaskAssignment
		want TaskStatus
	}{
		{"chua giao cho ai", nil, TaskWaitingAccept},
		{"giao roi chua nhan", []TaskAssignment{a(AssignPending)}, TaskWaitingAccept},
		{"da nhan", []TaskAssignment{a(AssignInProgress)}, TaskInProgress},
		{"tat ca xong", []TaskAssignment{a(AssignCompleted), a(AssignCompleted)}, TaskCompleted},
		{"1 nguoi tam dung", []TaskAssignment{a(AssignPaused)}, TaskPaused},
		{"1 nguoi treo", []TaskAssignment{a(AssignOnHold)}, TaskOnHold},
		{"co nguoi dang lam thi uu tien in_progress",
			[]TaskAssignment{a(AssignPaused), a(AssignInProgress)}, TaskInProgress},
		{"chia task: 1 xong 1 chua nhan",
			[]TaskAssignment{a(AssignCompleted), a(AssignPending)}, TaskInProgress},
		{"card bi thu hoi khong tinh",
			[]TaskAssignment{a(AssignRevoked), a(AssignPending)}, TaskWaitingAccept},
		{"chi con card thu hoi", []TaskAssignment{a(AssignRevoked)}, TaskWaitingAccept},
		{"treo + tam dung -> paused",
			[]TaskAssignment{a(AssignOnHold), a(AssignPaused)}, TaskPaused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveTaskStatus(tc.in); got != tc.want {
				t.Fatalf("DeriveTaskStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAssignmentTransitions(t *testing.T) {
	ok := [][2]AssignmentStatus{
		{AssignPending, AssignInProgress},   // nhan task
		{AssignInProgress, AssignPaused},    // tam dung
		{AssignPaused, AssignInProgress},    // tiep tuc
		{AssignInProgress, AssignOnHold},    // treo
		{AssignOnHold, AssignInProgress},    // tiep tuc sau khi treo
		{AssignInProgress, AssignCompleted}, // hoan thanh
		{AssignCompleted, AssignInProgress}, // mo lai khi co phieu sua
		{AssignPending, AssignRevoked},      // doi nguoi phu trach
	}
	for _, p := range ok {
		if !p[0].CanMoveTo(p[1]) {
			t.Errorf("phai cho phep %s -> %s", p[0], p[1])
		}
	}

	bad := [][2]AssignmentStatus{
		{AssignPending, AssignCompleted}, // chua nhan khong the hoan thanh
		{AssignPending, AssignPaused},
		{AssignCompleted, AssignCompleted},
		{AssignRevoked, AssignInProgress},
		{AssignPaused, AssignCompleted}, // phai tiep tuc truoc khi hoan thanh
	}
	for _, p := range bad {
		if p[0].CanMoveTo(p[1]) {
			t.Errorf("phai chan %s -> %s", p[0], p[1])
		}
	}
}

func TestDisplayRole(t *testing.T) {
	cases := []struct {
		name string
		user User
		want string
	}{
		{"admin cua CMS", User{Role: "user", CMSProfile: JSONMap{"botAdminOf": []any{1.0}}}, "admin"},
		{"tro ly nhom", User{Role: "user", CMSProfile: JSONMap{"assistantOf": []any{"wg_1"}}}, "assistant"},
		{"designer theo to Web", User{Role: "user", IsDesigner: true, CMSProfile: JSONMap{}}, "designer"},
		{"user CMS khong to nao", User{Role: "user", CMSProfile: JSONMap{}}, "order"},
		// Token dev / he thong auth khac: giu nguyen role trong token.
		{"token dev role designer", User{Role: "designer"}, "designer"},
		{"token dev role order", User{Role: "order"}, "order"},
		{"token dev khong co role", User{}, "order"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.DisplayRole(); got != tc.want {
				t.Errorf("DisplayRole() = %q, want %q", got, tc.want)
			}
		})
	}
}
