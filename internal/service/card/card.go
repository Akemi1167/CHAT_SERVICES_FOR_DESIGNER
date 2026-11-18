// Package card xu ly card abc.card/v1 do CMS dung.
//
// Chat KHONG dung card va khong biet nghiep vu don: no chi
//   - luu card + cardVersion,
//   - LOC nut theo visibleTo truoc khi tra cho UI,
//   - CHAN action ma nguoi bam khong duoc phep,
//   - chuyen event sang CMS va ap card moi CMS tra ve.
package card

import (
	"github.com/abc-workspace/chat-service/internal/domain"
)

const (
	keyVisibleTo = "visibleTo"
	keyAction    = "action"
	keyElements  = "elements"
)

// Visible cho biet node co hien voi actor khong.
// visibleTo vang mat / null / rong => hien voi moi nguoi (theo tai lieu CMS).
func Visible(node map[string]any, actorID string) bool {
	raw, ok := node[keyVisibleTo]
	if !ok || raw == nil {
		return true
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return true
	}
	for _, v := range list {
		if s, ok := v.(string); ok && s == actorID {
			return true
		}
	}
	return false
}

// FilterForViewer tra ban sao card chi gom phan tu ma actor duoc thay.
// Tra nil neu ca card khong hien voi nguoi nay.
func FilterForViewer(c domain.JSONMap, actorID string) domain.JSONMap {
	if c == nil {
		return nil
	}
	filtered, ok := filterValue(map[string]any(c), actorID).(map[string]any)
	if !ok {
		return nil
	}
	return domain.JSONMap(filtered)
}

func filterValue(v any, actorID string) any {
	switch node := v.(type) {
	case map[string]any:
		if !Visible(node, actorID) {
			return nil
		}
		out := make(map[string]any, len(node))
		for k, val := range node {
			if fv := filterValue(val, actorID); fv != nil {
				out[k] = fv
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(node))
		for _, item := range node {
			if fv := filterValue(item, actorID); fv != nil {
				out = append(out, fv)
			}
		}
		return out
	default:
		return v
	}
}

// FindAction tim node chua "action" = name trong card (duyet sau moi cap).
func FindAction(c domain.JSONMap, name string) (map[string]any, bool) {
	if c == nil || name == "" {
		return nil, false
	}
	return findAction(map[string]any(c), name)
}

func findAction(v any, name string) (map[string]any, bool) {
	switch node := v.(type) {
	case map[string]any:
		if a, ok := node[keyAction].(string); ok && a == name {
			return node, true
		}
		for _, val := range node {
			if found, ok := findAction(val, name); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range node {
			if found, ok := findAction(item, name); ok {
				return found, true
			}
		}
	}
	return nil, false
}

// CanAct: action phai ton tai trong card VA actor phai nam trong visibleTo cua nut
// (cung nhu cua card cha). Dung truoc khi gui event card.action sang CMS.
func CanAct(c domain.JSONMap, name, actorID string) bool {
	visible := FilterForViewer(c, actorID)
	if visible == nil {
		return false
	}
	_, ok := FindAction(visible, name)
	return ok
}

// Actions liet ke cac action actor duoc phep bam (tien cho UI/debug).
func Actions(c domain.JSONMap, actorID string) []string {
	visible := FilterForViewer(c, actorID)
	if visible == nil {
		return nil
	}
	out := make([]string, 0, 4)
	collectActions(map[string]any(visible), &out)
	return out
}

func collectActions(v any, out *[]string) {
	switch node := v.(type) {
	case map[string]any:
		if a, ok := node[keyAction].(string); ok && a != "" {
			*out = append(*out, a)
		}
		for _, val := range node {
			collectActions(val, out)
		}
	case []any:
		for _, item := range node {
			collectActions(item, out)
		}
	}
}
