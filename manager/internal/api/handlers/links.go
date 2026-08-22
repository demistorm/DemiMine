package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/demimine/manager/internal/docker"
	"github.com/go-chi/chi/v5"
)

// file links let one server/proxy share a file or dir with another. on disk
// it's a symlink (so host-side tools + backups see it), and at container
// create we shadow it with a real bind mount so the MC server sees a native
// file/dir instead of a broken symlink.
type FileLinkHandler struct {
	db *sql.DB
	// host-side base (for docker bind specs) and container-side base (for
	// actually touching the files from inside the manager)
	hostBase string
	diskBase string
}

func NewFileLinkHandler(db *sql.DB, hostServersDir string, serversDir string) *FileLinkHandler {
	return &FileLinkHandler{db: db, hostBase: hostServersDir, diskBase: serversDir}
}

type FileLink struct {
	ID            int64  `json:"id"`
	SourceType    string `json:"source_type"`
	SourceID      int64  `json:"source_id"`
	SourceName    string `json:"source_name"`
	SourcePath    string `json:"source_path"`
	TargetType    string `json:"target_type"`
	TargetID      int64  `json:"target_id"`
	TargetName    string `json:"target_name"`
	TargetPath    string `json:"target_path"`
	SourceMissing bool   `json:"source_missing"`
	SourceIsDir   bool   `json:"source_is_dir"`
}

type CreateLinkRequest struct {
	SourceType string `json:"source_type"`
	SourceID   int64  `json:"source_id"`
	SourcePath string `json:"source_path"`
	TargetType string `json:"target_type"`
	TargetID   int64  `json:"target_id"`
	TargetPath string `json:"target_path"`
}

// ---- path helpers (pure, unit tested) ----

// cleans "/plugins/Foo/" into "plugins/Foo"; empty result or anything that
// tries to escape is invalid
func normalizeLinkPath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", false
	}
	cleaned := filepath.Clean("/" + strings.TrimPrefix(p, "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.Contains(cleaned, "../") || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// true if p equals under or sits somewhere inside under
func pathIsUnder(under, p string) bool {
	return p == under || strings.HasPrefix(p, under+"/")
}

// ---- entity lookup ----

type linkEntity struct {
	id            int64
	name          string
	sanitizedName string
	status        string
}

func lookupLinkEntity(db *sql.DB, entType string, id int64) (*linkEntity, error) {
	if entType != "server" && entType != "proxy" {
		return nil, fmt.Errorf("invalid type %q", entType)
	}
	table := "servers"
	if entType == "proxy" {
		table = "proxies"
	}
	var e linkEntity
	e.id = id
	err := db.QueryRow(
		fmt.Sprintf("SELECT name, sanitized_name, status FROM %s WHERE id = ?", table), id,
	).Scan(&e.name, &e.sanitizedName, &e.status)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func hostPathFor(hostBase string, e *linkEntity, relPath string) string {
	return filepath.Join(hostBase, e.sanitizedName, filepath.FromSlash(relPath))
}

// ---- mounts for container create ----

// resolves all links targeting an entity into extra bind mounts. fail-open:
// a broken link just logs and skips so a missing source can't brick starts.
// diskBase is where the manager can stat the files; hostBase is what the
// docker daemon needs in the bind spec.
func LinksToMounts(db *sql.DB, diskBase, hostBase, entType string, entID int64) []docker.MountSpec {
	rows, err := db.Query(
		`SELECT target_path, source_type, source_id, source_path FROM file_links
		 WHERE target_type = ? AND target_id = ?`, entType, entID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	type pending struct {
		targetPath string
		sourceType string
		sourceID   int64
		sourcePath string
	}
	var pendings []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.targetPath, &p.sourceType, &p.sourceID, &p.sourcePath); err != nil {
			continue
		}
		pendings = append(pendings, p)
	}

	mountBase := "/server"
	if entType == "proxy" {
		mountBase = "/proxy"
	}

	var mounts []docker.MountSpec
	for _, p := range pendings {
		src, err := lookupLinkEntity(db, p.sourceType, p.sourceID)
		if err != nil {
			continue
		}
		mounts = append(mounts, docker.MountSpec{
			HostPath:      hostPathFor(hostBase, src, p.sourcePath),
			DiskPath:      hostPathFor(diskBase, src, p.sourcePath),
			ContainerPath: mountBase + "/" + p.targetPath,
		})
	}
	return mounts
}

// ---- HTTP handlers ----

func (h *FileLinkHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	sourcePath, ok := normalizeLinkPath(req.SourcePath)
	if !ok {
		http.Error(w, `{"error":"invalid source path"}`, http.StatusBadRequest)
		return
	}
	targetPath, ok := normalizeLinkPath(req.TargetPath)
	if !ok {
		http.Error(w, `{"error":"invalid target path"}`, http.StatusBadRequest)
		return
	}

	source, err := lookupLinkEntity(h.db, req.SourceType, req.SourceID)
	if err != nil {
		http.Error(w, `{"error":"source not found"}`, http.StatusNotFound)
		return
	}
	target, err := lookupLinkEntity(h.db, req.TargetType, req.TargetID)
	if err != nil {
		http.Error(w, `{"error":"target not found"}`, http.StatusNotFound)
		return
	}

	if req.SourceType == req.TargetType && req.SourceID == req.TargetID {
		http.Error(w, `{"error":"can't link a file to itself"}`, http.StatusBadRequest)
		return
	}

	if target.status == "running" {
		http.Error(w, `{"error":"target must be stopped before linking (restart applies the link)"}`, http.StatusConflict)
		return
	}

	srcFullPath := hostPathFor(h.diskBase, source, sourcePath)
	if _, err := os.Lstat(srcFullPath); err != nil {
		http.Error(w, `{"error":"source path does not exist"}`, http.StatusBadRequest)
		return
	}

	dstFullPath := hostPathFor(h.diskBase, target, targetPath)
	if _, err := os.Lstat(dstFullPath); err == nil {
		http.Error(w, `{"error":"target already has a file/dir with that name"}`, http.StatusConflict)
		return
	}
	dstDir := filepath.Dir(dstFullPath)
	if info, err := os.Stat(dstDir); err != nil || !info.IsDir() {
		http.Error(w, `{"error":"target parent folder does not exist"}`, http.StatusBadRequest)
		return
	}

	// no linking something that is itself a link (or lives inside one)
	if h.linkAt(req.SourceType, req.SourceID, sourcePath) != nil ||
		h.linkUnder(req.SourceType, req.SourceID, sourcePath) {
		http.Error(w, `{"error":"can't link a linked item — link the original instead"}`, http.StatusBadRequest)
		return
	}
	// no nesting a new link inside an existing mount
	if h.linkUnder(req.TargetType, req.TargetID, targetPath) {
		http.Error(w, `{"error":"can't link inside another linked folder"}`, http.StatusBadRequest)
		return
	}

	// relative symlink so the whole servers/ tree stays relocatable
	rel, err := filepath.Rel(dstDir, srcFullPath)
	if err != nil {
		http.Error(w, `{"error":"failed to compute link path"}`, http.StatusInternalServerError)
		return
	}
	if err := os.Symlink(rel, dstFullPath); err != nil {
		http.Error(w, `{"error":"failed to create link: "+err.Error()}`, http.StatusInternalServerError)
		return
	}

	result, err := h.db.Exec(
		`INSERT INTO file_links (source_type, source_id, source_path, target_type, target_id, target_path)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		req.SourceType, req.SourceID, sourcePath, req.TargetType, req.TargetID, targetPath)
	if err != nil {
		_ = os.Remove(dstFullPath)
		http.Error(w, `{"error":"failed to save link"}`, http.StatusInternalServerError)
		return
	}
	linkID, _ := result.LastInsertId()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      linkID,
		"success": true,
	})
}

func (h *FileLinkHandler) List(w http.ResponseWriter, r *http.Request) {
	entType := r.URL.Query().Get("target_type")
	entIDStr := r.URL.Query().Get("target_id")

	query := `SELECT l.id, l.source_type, l.source_id, l.source_path, l.target_type, l.target_id, l.target_path FROM file_links l`
	var args []interface{}
	if entType != "" && entIDStr != "" {
		var entID int64
		if _, err := fmt.Sscanf(entIDStr, "%d", &entID); err != nil {
			http.Error(w, `{"error":"invalid target_id"}`, http.StatusBadRequest)
			return
		}
		query += ` WHERE l.target_type = ? AND l.target_id = ?`
		args = append(args, entType, entID)
	}
	query += ` ORDER BY l.target_path`

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, `{"error":"failed to list links"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	links := []FileLink{}
	for rows.Next() {
		var l FileLink
		if err := rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourcePath, &l.TargetType, &l.TargetID, &l.TargetPath); err != nil {
			continue
		}
		if src, err := lookupLinkEntity(h.db, l.SourceType, l.SourceID); err == nil {
			l.SourceName = src.name
		}
		if tgt, err := lookupLinkEntity(h.db, l.TargetType, l.TargetID); err == nil {
			l.TargetName = tgt.name
		}
		if src, err := lookupLinkEntity(h.db, l.SourceType, l.SourceID); err == nil {
			if info, err := os.Lstat(hostPathFor(h.diskBase, src, l.SourcePath)); err != nil {
				l.SourceMissing = true
			} else {
				l.SourceIsDir = info.IsDir()
			}
		} else {
			l.SourceMissing = true
		}
		links = append(links, l)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"links": links})
}

// delete button — removes the link, source data untouched
func (h *FileLinkHandler) Delete(w http.ResponseWriter, r *http.Request) {
	linkID, err := scanChiID(r)
	if err != nil {
		http.Error(w, `{"error":"invalid link id"}`, http.StatusBadRequest)
		return
	}

	l, target, ok := h.loadLink(linkID)
	if !ok {
		http.Error(w, `{"error":"link not found"}`, http.StatusNotFound)
		return
	}
	if target.status == "running" {
		http.Error(w, `{"error":"stop the server/proxy before removing the link"}`, http.StatusConflict)
		return
	}

	h.removeLinkOnDisk(l)
	if _, err := h.db.Exec("DELETE FROM file_links WHERE id = ?", linkID); err != nil {
		http.Error(w, `{"error":"failed to remove link"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// unlink button — swap the link for a local copy of the data
func (h *FileLinkHandler) Unlink(w http.ResponseWriter, r *http.Request) {
	linkID, err := scanChiID(r)
	if err != nil {
		http.Error(w, `{"error":"invalid link id"}`, http.StatusBadRequest)
		return
	}

	l, target, ok := h.loadLink(linkID)
	if !ok {
		http.Error(w, `{"error":"link not found"}`, http.StatusNotFound)
		return
	}
	if target.status == "running" {
		http.Error(w, `{"error":"stop the server/proxy before unlinking"}`, http.StatusConflict)
		return
	}

	source, err := lookupLinkEntity(h.db, l.SourceType, l.SourceID)
	if err != nil {
		http.Error(w, `{"error":"source no longer exists — use remove instead"}`, http.StatusConflict)
		return
	}
	srcFullPath := hostPathFor(h.diskBase, source, l.SourcePath)
	if _, err := os.Lstat(srcFullPath); err != nil {
		http.Error(w, `{"error":"source file is gone — use remove instead"}`, http.StatusConflict)
		return
	}

	dstFullPath := hostPathFor(h.diskBase, target, l.TargetPath)

	// copy to a temp sibling first so a failed copy never leaves a half state
	tmpPath := dstFullPath + ".demimine-copy"
	_ = os.RemoveAll(tmpPath)
	if err := copyPath(srcFullPath, tmpPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		http.Error(w, `{"error":"failed to copy: "+err.Error()}`, http.StatusInternalServerError)
		return
	}
	h.removeLinkOnDisk(l)
	if err := os.Rename(tmpPath, dstFullPath); err != nil {
		_ = os.RemoveAll(tmpPath)
		http.Error(w, `{"error":"failed to place local copy"}`, http.StatusInternalServerError)
		return
	}
	if _, err := h.db.Exec("DELETE FROM file_links WHERE id = ?", linkID); err != nil {
		http.Error(w, `{"error":"failed to remove link"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// ---- shared helpers used by other handlers ----

func scanChiID(r *http.Request) (int64, error) {
	var id int64
	_, err := fmt.Sscanf(chi.URLParam(r, "id"), "%d", &id)
	return id, err
}

func (h *FileLinkHandler) loadLink(linkID int64) (*FileLink, *linkEntity, bool) {
	var l FileLink
	err := h.db.QueryRow(
		`SELECT id, source_type, source_id, source_path, target_type, target_id, target_path FROM file_links WHERE id = ?`, linkID,
	).Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourcePath, &l.TargetType, &l.TargetID, &l.TargetPath)
	if err != nil {
		return nil, nil, false
	}
	target, err := lookupLinkEntity(h.db, l.TargetType, l.TargetID)
	if err != nil {
		return nil, nil, false
	}
	return &l, target, true
}

// removes the symlink at the target if it's still a symlink — never touches
// real data that somehow ended up at that path
func (h *FileLinkHandler) removeLinkOnDisk(l *FileLink) {
	target, err := lookupLinkEntity(h.db, l.TargetType, l.TargetID)
	if err != nil {
		return
	}
	linkPath := hostPathFor(h.diskBase, target, l.TargetPath)
	if info, err := os.Lstat(linkPath); err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(linkPath)
	}
}

// exact link registered at this path?
func (h *FileLinkHandler) linkAt(entType string, entID int64, relPath string) *FileLink {
	var l FileLink
	err := h.db.QueryRow(
		`SELECT id, source_type, source_id, source_path, target_type, target_id, target_path
		 FROM file_links WHERE target_type = ? AND target_id = ? AND target_path = ?`,
		entType, entID, relPath,
	).Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourcePath, &l.TargetType, &l.TargetID, &l.TargetPath)
	if err != nil {
		return nil
	}
	return &l
}

// any link registered at or under this path?
func (h *FileLinkHandler) linkUnder(entType string, entID int64, relPath string) bool {
	rows, err := h.db.Query(
		`SELECT target_path FROM file_links WHERE target_type = ? AND target_id = ?`, entType, entID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil && pathIsUnder(relPath, p) {
			return true
		}
	}
	return false
}

// links whose source is this path (or under it) — i.e. deleting/renaming
// this path would break other servers
func (h *FileLinkHandler) dependentsOnSource(entType string, entID int64, relPath string) []FileLink {
	rows, err := h.db.Query(
		`SELECT id, source_type, source_id, source_path, target_type, target_id, target_path
		 FROM file_links WHERE source_type = ? AND source_id = ?`, entType, entID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []FileLink
	for rows.Next() {
		var l FileLink
		if err := rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourcePath, &l.TargetType, &l.TargetID, &l.TargetPath); err != nil {
			continue
		}
		if pathIsUnder(relPath, l.SourcePath) {
			if src, err := lookupLinkEntity(h.db, l.SourceType, l.SourceID); err == nil {
				l.SourceName = src.name
			}
			if tgt, err := lookupLinkEntity(h.db, l.TargetType, l.TargetID); err == nil {
				l.TargetName = tgt.name
			}
			out = append(out, l)
		}
	}
	return out
}

// delete rows for links that lived under a deleted path (the symlinks
// themselves die with the RemoveAll that already ran)
func (h *FileLinkHandler) sweepLinksUnderPath(entType string, entID int64, relPath string) {
	links := h.linksUnder(entType, entID, relPath)
	for _, l := range links {
		h.db.Exec("DELETE FROM file_links WHERE id = ?", l.ID)
	}
}

func (h *FileLinkHandler) linksUnder(entType string, entID int64, relPath string) []FileLink {
	rows, err := h.db.Query(
		`SELECT id, source_type, source_id, source_path, target_type, target_id, target_path
		 FROM file_links WHERE target_type = ? AND target_id = ?`, entType, entID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []FileLink
	for rows.Next() {
		var l FileLink
		if err := rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourcePath, &l.TargetType, &l.TargetID, &l.TargetPath); err != nil {
			continue
		}
		if pathIsUnder(relPath, l.TargetPath) {
			out = append(out, l)
		}
	}
	return out
}

// called when a server or proxy is deleted: drop every link row pointing at
// it, and detach anything that was linking FROM it so no dangling symlinks
// stay behind in other servers
func (h *FileLinkHandler) CleanupEntityDeleted(entType string, entID int64) {
	rows, err := h.db.Query(
		`SELECT id, source_type, source_id, source_path, target_type, target_id, target_path
		 FROM file_links WHERE source_type = ? AND source_id = ?`, entType, entID)
	if err != nil {
		return
	}
	var sources []FileLink
	for rows.Next() {
		var l FileLink
		if err := rows.Scan(&l.ID, &l.SourceType, &l.SourceID, &l.SourcePath, &l.TargetType, &l.TargetID, &l.TargetPath); err == nil {
			sources = append(sources, l)
		}
	}
	rows.Close()

	for _, l := range sources {
		h.removeLinkOnDisk(&l)
		h.db.Exec("DELETE FROM file_links WHERE id = ?", l.ID)
	}

	// links INTO the deleted entity die with its directory
	h.db.Exec("DELETE FROM file_links WHERE target_type = ? AND target_id = ?", entType, entID)
}

// ---- copy helper ----

func copyPath(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode())
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return nil // skip symlinks/sockets inside the tree
		}
		return copyFile(path, target, fi.Mode())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
