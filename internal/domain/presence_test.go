package domain

import (
	"testing"
	"time"
)

func userAt(p Presence, seen *time.Time) *User {
	return &User{Presence: p, LastSeenAt: seen}
}

func ago(d time.Duration) *time.Time {
	t := time.Now().UTC().Add(-d)
	return &t
}

// Day la cai chong "online mac ket" khi process bi kill.
func TestLivePresence(t *testing.T) {
	SetPresenceTTL(90 * time.Second)
	t.Cleanup(func() { SetPresenceTTL(DefaultPresenceTTL) })

	cases := []struct {
		name string
		user *User
		want Presence
	}{
		{"online, vua cham 10s truoc", userAt(PresenceOnline, ago(10*time.Second)), PresenceOnline},
		{"online, cham 89s truoc (con trong nguong)", userAt(PresenceOnline, ago(89*time.Second)), PresenceOnline},
		{"online nhung cham 2 phut truoc -> coi la offline",
			userAt(PresenceOnline, ago(2*time.Minute)), PresenceOffline},
		{"online nhung cham tu hom qua -> offline",
			userAt(PresenceOnline, ago(24*time.Hour)), PresenceOffline},
		{"online ma chua bao gio cham -> offline",
			userAt(PresenceOnline, nil), PresenceOffline},

		// off_duty do CMS dat theo lich nghi, KHONG phu thuoc ket noi nen khong het han.
		{"off_duty tu hom qua -> giu off_duty",
			userAt(PresenceOffDuty, ago(24*time.Hour)), PresenceOffDuty},
		{"off_duty khong co last_seen -> giu off_duty",
			userAt(PresenceOffDuty, nil), PresenceOffDuty},

		{"offline thi giu offline", userAt(PresenceOffline, ago(time.Second)), PresenceOffline},
		{"user nil", nil, PresenceOffline},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.user.LivePresence(); got != c.want {
				t.Errorf("LivePresence() = %q, muon %q", got, c.want)
			}
		})
	}
}

// IsOnline phai khop LivePresence, khong doc truc tiep cot Presence.
func TestIsOnline(t *testing.T) {
	SetPresenceTTL(90 * time.Second)
	t.Cleanup(func() { SetPresenceTTL(DefaultPresenceTTL) })

	if !userAt(PresenceOnline, ago(5*time.Second)).IsOnline() {
		t.Error("vua cham thi phai online")
	}
	if userAt(PresenceOnline, ago(10*time.Minute)).IsOnline() {
		t.Error("cham 10 phut truoc thi KHONG con online")
	}
}

// TTL <= 0 = tat viec coi la het han (luon tin cot presence).
func TestTTLBangKhongThiTatHetHan(t *testing.T) {
	SetPresenceTTL(0)
	t.Cleanup(func() { SetPresenceTTL(DefaultPresenceTTL) })

	if got := userAt(PresenceOnline, ago(24*time.Hour)).LivePresence(); got != PresenceOnline {
		t.Errorf("TTL=0 thi phai giu online, nhan %q", got)
	}
	if got := userAt(PresenceOnline, nil).LivePresence(); got != PresenceOnline {
		t.Errorf("TTL=0 + khong co last_seen thi phai giu online, nhan %q", got)
	}
}

// Mac dinh phai dam bao TTL >= 3 lan heartbeat: mat 1-2 nhip chua bi coi offline.
func TestMacDinhTTLDuRongSoVoiHeartbeat(t *testing.T) {
	if DefaultPresenceTTL < 3*DefaultPresenceHeartbeat {
		t.Errorf("TTL mac dinh %v < 3x heartbeat %v — mat 1 nhip la bao offline oan",
			DefaultPresenceTTL, DefaultPresenceHeartbeat)
	}
}
