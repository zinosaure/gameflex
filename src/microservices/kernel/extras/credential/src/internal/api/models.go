package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// now mirrors the Python defaults: a naive local timestamp, the same shape
// SQLAlchemy stored.
func now() string {
	return time.Now().Format("2006-01-02 15:04:05.999999")
}

// ── Row shapes (mirror the Python SchemaResponse models exactly) ──

type userResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	RoleLevel   int    `json:"role_level"`
	CreatedAt   any    `json:"created_at"`
	CreatedByID any    `json:"created_by_id"`
	// Consoles — ce que le compte peut lancer (chantier 20) : `*` tout,
	// `retro` tout le rétro, puis des consoles une à une ; vide, aucune. Un
	// administrateur a toujours `*`. Voir gameflex/consoles, grants.go.
	Consoles []string `json:"consoles"`
	// Token — son jeton de fédération, pour la liste de l'administrateur
	// seulement (account_tokens.go).
	Token *accountTokenResponse `json:"token,omitempty"`
}

type historyResponse struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	ConsoleID   string `json:"console_id"`
	Reference   string `json:"reference"`
	Title       any    `json:"title"`
	GoSessionID any    `json:"go_session_id"`
	StartedAt   any    `json:"started_at"`
	EndedAt     any    `json:"ended_at"`
}

type controllerMappingResponse struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	ConsoleID string `json:"console_id"`
	Keyboard  any    `json:"keyboard"`
	Gamepad   any    `json:"gamepad"`
	UpdatedAt any    `json:"updated_at"`
}

type keyMappingResponse struct {
	ID                 int64           `json:"id"`
	UserID             int64           `json:"user_id"`
	PhysicalDeviceID   string          `json:"physical_device_id"`
	PhysicalDeviceType string          `json:"physical_device_type"`
	ControllerName     string          `json:"controller_name"`
	Mapping            json.RawMessage `json:"mapping"`
	UpdatedAt          any             `json:"updated_at"`
}

type keyPositioningResponse struct {
	ID             int64           `json:"id"`
	UserID         int64           `json:"user_id"`
	ControllerName string          `json:"controller_name"`
	SlotIndex      int             `json:"slot_index"`
	IsDefault      bool            `json:"is_default"`
	Layout         json.RawMessage `json:"layout"`
	Opacity        float64         `json:"opacity"`
	UpdatedAt      any             `json:"updated_at"`
}

type notificationPreferenceResponse struct {
	NotificationType string `json:"notification_type"`
	Enabled          bool   `json:"enabled"`
}

// isoDatetime renders SQLite datetime strings the way Pydantic's
// model_dump(mode="json") did: naive ISO 8601, ".000000" fractional seconds
// trimmed, NULL stays null.
// text — une colonne de TEXTE nullable, rendue telle quelle.
//
// Elle existe parce que son absence a coûté : les titres de jeux et les
// identifiants de session passaient par isoDatetime, le formateur de DATES.
// Celui-ci remplace la première espace par « T » et rogne les zéros après un
// point — parfait pour « 2026-09-01 12:00:00 », désastreux pour un titre.
//
// « Banjo Kazooie » sortait donc « BanjoTKazooie », et « Sonic 3D Blast 2000 »
// aurait perdu ses zéros. Constaté le 2026-09-01 sur les favoris et sur le
// « reprends là où tu t'étais arrêté » du tableau de bord.
//
// Deux types de données, deux fonctions : une valeur qui n'est pas une date
// ne doit pas traverser un formateur de dates.
func text(raw sql.NullString) any {
	if !raw.Valid {
		return nil
	}

	return raw.String
}

func isoDatetime(raw sql.NullString) any {
	if !raw.Valid {
		return nil
	}
	value := raw.String
	value = strings.Replace(value, " ", "T", 1)
	if idx := strings.Index(value, "."); idx >= 0 {
		value = strings.TrimRight(value, "0")
		if strings.HasSuffix(value, ".") {
			value = strings.TrimSuffix(value, ".")
		}
	}
	return value
}

func scanUser(rows interface{ Scan(...any) error }) (userResponse, error) {
	var user userResponse
	var createdAt sql.NullString
	var createdBy sql.NullInt64
	var grants string
	err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.RoleLevel, &createdAt, &createdBy, &grants)
	if err != nil {
		return user, err
	}
	user.Consoles = effectiveGrants(user.RoleLevel, grants)
	user.CreatedAt = isoDatetime(createdAt)
	if createdBy.Valid {
		user.CreatedByID = createdBy.Int64
	}
	return user, nil
}

// ── search parsing (libs/skeleton.py build_statement subset) ──
//
// search is a JSON object (one AND group) or array of objects (OR groups).
// Fields not in the allowed set are ignored, like Python's
// column_names ∩ search_fields. Returns a WHERE fragment and its args.

func parseSearch(raw, allowedColumns string) (string, []any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, nil
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return "", nil, fmt.Errorf("invalid search parameter")
	}

	allowed := map[string]bool{}
	for _, column := range strings.Split(allowedColumns, ",") {
		allowed[column] = true
	}

	var groups []map[string]any
	switch value := decoded.(type) {
	case map[string]any:
		groups = []map[string]any{value}
	case []any:
		for _, item := range value {
			if group, ok := item.(map[string]any); ok {
				groups = append(groups, group)
			}
		}
	default:
		return "", nil, fmt.Errorf("invalid search parameter")
	}

	var clauses []string
	var args []any
	for _, group := range groups {
		var groupClauses []string
		for field, rawCondition := range group {
			if !allowed[field] {
				continue
			}
			op, value := "=", rawCondition
			if condition, ok := rawCondition.(map[string]any); ok {
				for operator, operand := range condition {
					op, value = strings.ToLower(operator), operand
					break
				}
			}
			clause, clauseArgs, ok := buildCondition(field, op, value)
			if !ok {
				continue
			}
			groupClauses = append(groupClauses, clause)
			args = append(args, clauseArgs...)
		}
		if len(groupClauses) > 0 {
			clauses = append(clauses, "("+strings.Join(groupClauses, " AND ")+")")
		}
	}
	return strings.Join(clauses, " OR "), args, nil
}

func buildCondition(column, operator string, value any) (string, []any, bool) {
	switch operator {
	case "=", "==":
		return fmt.Sprintf("%s = ?", column), []any{value}, true
	case "!=":
		return fmt.Sprintf("%s != ?", column), []any{value}, true
	case ">":
		return fmt.Sprintf("%s > ?", column), []any{value}, true
	case "<":
		return fmt.Sprintf("%s < ?", column), []any{value}, true
	case ">=":
		return fmt.Sprintf("%s >= ?", column), []any{value}, true
	case "<=":
		return fmt.Sprintf("%s <= ?", column), []any{value}, true
	case "like":
		return fmt.Sprintf("%s LIKE ?", column), []any{"%" + fmt.Sprint(value) + "%"}, true
	case "not_like":
		return fmt.Sprintf("%s NOT LIKE ?", column), []any{"%" + fmt.Sprint(value) + "%"}, true
	case "startwith":
		return fmt.Sprintf("%s LIKE ?", column), []any{fmt.Sprint(value) + "%"}, true
	case "endwith":
		return fmt.Sprintf("%s LIKE ?", column), []any{"%" + fmt.Sprint(value)}, true
	case "in":
		list := toList(value)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(list)), ",")
		return fmt.Sprintf("%s IN (%s)", column, placeholders), list, len(list) > 0
	case "not_in":
		list := toList(value)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(list)), ",")
		return fmt.Sprintf("%s NOT IN (%s)", column, placeholders), list, len(list) > 0
	case "between":
		list := toList(value)
		if len(list) != 2 {
			return "", nil, false
		}
		return fmt.Sprintf("%s BETWEEN ? AND ?", column), list, true
	case "null":
		return fmt.Sprintf("%s IS NULL", column), nil, true
	case "not_null":
		return fmt.Sprintf("%s IS NOT NULL", column), nil, true
	default:
		return "", nil, false
	}
}

func toList(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}
