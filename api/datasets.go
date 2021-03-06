package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	util "github.com/datatogether/api/apiutil"
	"github.com/qri-io/dataset"
	"github.com/qri-io/dataset/dsutil"
	"github.com/qri-io/qri/actions"
	"github.com/qri-io/qri/lib"
	"github.com/qri-io/qri/p2p"
	"github.com/qri-io/qri/repo"
	"github.com/qri-io/qri/repo/profile"
	"github.com/qri-io/qri/rev"
)

// DatasetHandlers wraps a requests struct to interface with http.HandlerFunc
type DatasetHandlers struct {
	lib.DatasetRequests
	node     *p2p.QriNode
	repo     repo.Repo
	ReadOnly bool
}

// NewDatasetHandlers allocates a DatasetHandlers pointer
func NewDatasetHandlers(node *p2p.QriNode, readOnly bool) *DatasetHandlers {
	req := lib.NewDatasetRequests(node, nil)
	h := DatasetHandlers{*req, node, node.Repo, readOnly}
	return &h
}

// ListHandler is a dataset list endpoint
func (h *DatasetHandlers) ListHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "GET":
		if h.ReadOnly {
			readOnlyResponse(w, "/list")
			return
		}
		h.listHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// SaveHandler is a dataset save/update endpoint
func (h *DatasetHandlers) SaveHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "PUT", "POST":
		h.saveHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// RemoveHandler is a a dataset delete endpoint
func (h *DatasetHandlers) RemoveHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "DELETE", "POST":
		h.removeHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// GetHandler is a dataset single endpoint
func (h *DatasetHandlers) GetHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "GET":
		if h.ReadOnly {
			readOnlyResponse(w, "/me/")
			return
		}
		h.getHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// DiffHandler is a dataset single endpoint
func (h *DatasetHandlers) DiffHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "POST", "GET":
		if h.ReadOnly {
			readOnlyResponse(w, "/diff")
			return
		}
		h.diffHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// PeerListHandler is a dataset list endpoint
func (h *DatasetHandlers) PeerListHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "GET":
		h.peerListHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// AddHandler is an endpoint for creating new datasets
func (h *DatasetHandlers) AddHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "POST", "PUT":
		h.addHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// RenameHandler is the endpoint for renaming datasets
func (h *DatasetHandlers) RenameHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "POST", "PUT":
		h.renameHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// BodyHandler gets the contents of a dataset
func (h *DatasetHandlers) BodyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "GET":
		if h.ReadOnly {
			readOnlyResponse(w, "/body/")
			return
		}
		h.bodyHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// UnpackHandler unpacks a zip file and sends it back as json
func (h *DatasetHandlers) UnpackHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "POST":
		postData, err := ioutil.ReadAll(r.Body)
		if err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, err)
			return
		}
		h.unpackHandler(w, r, postData)
	default:
		util.NotFoundHandler(w, r)
	}
}

// PublishHandler works with dataset publicity
func (h *DatasetHandlers) PublishHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "GET":
		h.listPublishedHandler(w, r)
	case "POST":
		h.publishHandler(w, r, true)
	case "DELETE":
		h.publishHandler(w, r, false)
	default:
		util.NotFoundHandler(w, r)
	}
}

// UpdateHandler brings a dataset to the latest version
func (h *DatasetHandlers) UpdateHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "POST":
		h.updateHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

// ZipDatasetHandler is the endpoint for getting a zip archive of a dataset
func (h *DatasetHandlers) ZipDatasetHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "OPTIONS":
		util.EmptyOkHandler(w, r)
	case "GET":
		if h.ReadOnly {
			readOnlyResponse(w, "/export/")
			return
		}
		h.zipDatasetHandler(w, r)
	default:
		util.NotFoundHandler(w, r)
	}
}

func (h *DatasetHandlers) zipDatasetHandler(w http.ResponseWriter, r *http.Request) {
	ref := HTTPPathToQriPath(r.URL.Path[len("/export"):])
	format := r.FormValue("format")
	zipped := r.FormValue("zipped") == "true"
	tmpDir, err := ioutil.TempDir(os.TempDir(), "api_export")
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	params := lib.ExportParams{Ref: ref, TargetDir: tmpDir, Format: format, Zipped: zipped}

	var fileWritten string
	req := lib.NewExportRequests(h.node, nil)
	err = req.Export(&params, &fileWritten)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	f, err := os.Open(fileWritten)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	bytes, err := ioutil.ReadAll(f)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", extensionToMimeType(path.Ext(fileWritten)))
	w.Header().Set("Content-Disposition", fileWritten)
	w.Write(bytes)
}

func extensionToMimeType(ext string) string {
	switch ext {
	case ".json":
		return "application/json"
	case ".yaml":
		return "application/x-yaml"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".zip":
		return "application/zip"
	default:
		return ""
	}
}

func (h *DatasetHandlers) listHandler(w http.ResponseWriter, r *http.Request) {
	args := lib.ListParamsFromRequest(r)
	args.OrderBy = "created"

	res := []repo.DatasetRef{}
	if err := h.List(&args, &res); err != nil {
		log.Infof("error listing datasets: %s", err.Error())
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	if err := util.WritePageResponse(w, res, r, args.Page()); err != nil {
		log.Infof("error list datasests response: %s", err.Error())
	}
}

func (h *DatasetHandlers) listPublishedHandler(w http.ResponseWriter, r *http.Request) {
	args := lib.ListParamsFromRequest(r)
	args.OrderBy = "created"
	args.Published = true

	res := []repo.DatasetRef{}
	if err := h.List(&args, &res); err != nil {
		log.Infof("error listing datasets: %s", err.Error())
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	if err := util.WritePageResponse(w, res, r, args.Page()); err != nil {
		log.Infof("error list datasests response: %s", err.Error())
	}
}

func (h *DatasetHandlers) getHandler(w http.ResponseWriter, r *http.Request) {
	p := lib.GetParams{
		Path: HTTPPathToQriPath(r.URL.Path),
	}
	res := lib.GetResult{}
	err := h.Get(&p, &res)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	util.WriteResponse(w, res.Dataset)
}

func (h *DatasetHandlers) diffHandler(w http.ResponseWriter, r *http.Request) {
	req := &lib.DiffParams{}
	switch r.Header.Get("Content-Type") {
	case "application/json":
		if err := json.NewDecoder(r.Body).Decode(req); err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("error decoding body into params: %s", err.Error()))
			return
		}
	default:
		req = &lib.DiffParams{
			LeftPath:  r.FormValue("left_path"),
			RightPath: r.FormValue("right_path"),
			Selector:  r.FormValue("selector"),
		}
	}

	res := &lib.DiffResponse{}
	if err := h.Diff(req, res); err != nil {
		fmt.Println(err)
		util.WriteErrResponse(w, http.StatusInternalServerError, fmt.Errorf("error generating diff: %s", err.Error()))
		return
	}

	env := map[string]interface{}{
		"meta": map[string]interface{}{
			"code": http.StatusOK,
			"stat": res.Stat,
		},
		"data": res.Diff,
	}

	resData, err := json.Marshal(env)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(resData)
}

func (h *DatasetHandlers) peerListHandler(w http.ResponseWriter, r *http.Request) {
	log.Info(r.URL.Path)
	p := lib.ListParamsFromRequest(r)
	p.OrderBy = "created"

	// TODO - cheap peerId detection
	profileID := r.URL.Path[len("/list/"):]
	if len(profileID) > 0 && profileID[:2] == "Qm" {
		// TODO - let's not ignore this error
		p.ProfileID, _ = profile.IDB58Decode(profileID)
	} else {
		ref, err := DatasetRefFromPath(r.URL.Path[len("/list/"):])
		if err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, err)
			return
		}
		if !ref.IsPeerRef() {
			util.WriteErrResponse(w, http.StatusBadRequest, errors.New("request needs to be in the form '/list/[peername]'"))
			return
		}
		p.Peername = ref.Peername
	}

	res := []repo.DatasetRef{}
	if err := h.List(&p, &res); err != nil {
		log.Infof("error listing peer's datasets: %s", err.Error())
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	if err := util.WritePageResponse(w, res, r, p.Page()); err != nil {
		log.Infof("error list datasests response: %s", err.Error())
	}
}

// when datasets are created with save/new dataset bodies they can be run with "return body",
// which populates res.Dataset.Body with a qfs.File of raw data
// addBodyFile sets the dataset body, converting to JSON for a response the API can understand
// TODO - make this less bad. this should happen lower and lib Params should be used to set the response
// body to well-formed JSON
func addBodyFile(res *repo.DatasetRef) error {
	file := res.Dataset.BodyFile()
	if file == nil {
		log.Error("no body file")
		return fmt.Errorf("no response body file")
	}

	if res.Dataset.Structure.Format == dataset.JSONDataFormat.String() {
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return err
		}
		res.Dataset.Body = json.RawMessage(data)
		return nil
	}

	in := res.Dataset.Structure
	st := &dataset.Structure{}
	st.Assign(in, &dataset.Structure{
		Format: "json",
		Schema: in.Schema,
	})

	data, err := actions.ConvertBodyFile(file, in, st, 0, 0, true)
	if err != nil {
		log.Errorf("converting body file to JSON: %s", err)
		return fmt.Errorf("converting body file to JSON: %s", err)
	}
	res.Dataset.Body = json.RawMessage(data)
	return nil
}

func (h *DatasetHandlers) addHandler(w http.ResponseWriter, r *http.Request) {
	ref, err := DatasetRefFromPath(r.URL.Path[len("/add"):])
	if err != nil {
		util.WriteErrResponse(w, http.StatusBadRequest, err)
		return
	}

	if ref.Peername == "" || ref.Name == "" {
		util.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("need peername and dataset name: '/add/[peername]/[datasetname]'"))
		return
	}

	res := repo.DatasetRef{}
	err = h.Add(&ref, &res)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	util.WriteResponse(w, res)
}

func (h *DatasetHandlers) saveHandler(w http.ResponseWriter, r *http.Request) {
	ds := &dataset.Dataset{}

	if r.Header.Get("Content-Type") == "application/json" {
		err := json.NewDecoder(r.Body).Decode(ds)
		if err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, err)
			return
		}

		if strings.Contains(r.URL.Path, "/save/") {
			args, err := DatasetRefFromPath(r.URL.Path[len("/save/"):])
			if err != nil {
				util.WriteErrResponse(w, http.StatusBadRequest, err)
				return
			}
			if args.Peername != "" {
				ds.Peername = args.Peername
				ds.Name = args.Name
			}
		}
	} else {
		if err := dsutil.FormFileDataset(r, ds); err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, err)
			return
		}
	}

	res := &repo.DatasetRef{}
	scriptOutput := &bytes.Buffer{}
	p := &lib.SaveParams{
		Dataset:             ds,
		Private:             r.FormValue("private") == "true",
		DryRun:              r.FormValue("dry_run") == "true",
		ReturnBody:          r.FormValue("return_body") == "true",
		Force:               r.FormValue("force") == "true",
		ConvertFormatToPrev: true,
		ScriptOutput:        scriptOutput,
	}

	if r.FormValue("secrets") != "" {
		p.Secrets = map[string]string{}
		if err := json.Unmarshal([]byte(r.FormValue("secrets")), &p.Secrets); err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("parsing secrets: %s", err))
			return
		}
	} else if ds.Transform != nil && ds.Transform.Secrets != nil {
		// TODO remove this, require API consumers to send secrets separately
		p.Secrets = ds.Transform.Secrets
	}

	if err := h.Save(p, res); err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	// Don't leak paths across the API, it's possible they contain absolute paths or tmp dirs.
	res.Dataset.BodyPath = filepath.Base(res.Dataset.BodyPath)

	if p.ReturnBody {
		if err := addBodyFile(res); err != nil {
			util.WriteErrResponse(w, http.StatusInternalServerError, err)
			return
		}
	}

	msg := scriptOutput.String()
	util.WriteMessageResponse(w, msg, res)
}

func (h *DatasetHandlers) removeHandler(w http.ResponseWriter, r *http.Request) {
	p := lib.RemoveParams{
		Ref:      HTTPPathToQriPath(r.URL.Path[len("/remove"):]),
		Revision: rev.Rev{Field: "ds", Gen: -1},
	}
	res := lib.RemoveResponse{}
	if err := h.Remove(&p, &res); err != nil {
		log.Infof("error deleting dataset: %s", err.Error())
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	util.WriteResponse(w, res)
}

// RenameReqParams is an encoding struct
// its intent is to be a more user-friendly structure for the api endpoint
// that will map to and from the lib.RenameParams struct
type RenameReqParams struct {
	Current string
	New     string
}

func (h DatasetHandlers) renameHandler(w http.ResponseWriter, r *http.Request) {
	reqParams := &RenameReqParams{}
	p := &lib.RenameParams{}
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(reqParams); err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, err)
			return
		}
	} else {
		reqParams.Current = r.URL.Query().Get("current")
		reqParams.New = r.URL.Query().Get("new")
	}
	current, err := repo.ParseDatasetRef(reqParams.Current)
	if err != nil {
		util.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("error parsing current param: %s", err.Error()))
		return
	}
	n, err := repo.ParseDatasetRef(reqParams.New)
	if err != nil {
		util.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("error parsing new param: %s", err.Error()))
		return
	}
	p = &lib.RenameParams{
		Current: current,
		New:     n,
	}

	res := &repo.DatasetRef{}
	if err := h.Rename(p, res); err != nil {
		log.Infof("error renaming dataset: %s", err.Error())
		util.WriteErrResponse(w, http.StatusBadRequest, err)
		return
	}

	util.WriteResponse(w, res)
}

func loadFileIfPath(path string) (file *os.File, err error) {
	if path == "" {
		return nil, nil
	}

	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("filepath must be absolute")
	}

	return os.Open(path)
}

// default number of entries to limit to when reading
// TODO - should move this into lib
const defaultDataLimit = 100

// DataResponse is the struct used to respond to api requests made to the /data endpoint
// It is necessary because we need to include the 'path' field in the response
type DataResponse struct {
	Path string          `json:"path"`
	Data json.RawMessage `json:"data"`
}

func (h DatasetHandlers) bodyHandler(w http.ResponseWriter, r *http.Request) {
	d, err := DatasetRefFromPath(r.URL.Path[len("/body"):])
	if err != nil {
		util.WriteErrResponse(w, http.StatusBadRequest, err)
		return
	}

	err = repo.CanonicalizeDatasetRef(h.repo, &d)
	if err != nil && err != repo.ErrNotFound {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	limit, err := util.ReqParamInt("limit", r)
	if err != nil {
		limit = defaultDataLimit
		err = nil
	}
	offset, err := util.ReqParamInt("offset", r)
	if err != nil {
		offset = 0
		err = nil
	}

	p := &lib.GetParams{
		Path:     d.String(),
		Format:   "json",
		Selector: "body",
		Limit:    limit,
		Offset:   offset,
		All:      r.FormValue("all") == "true" && limit == defaultDataLimit && offset == 0,
	}

	result := &lib.GetResult{}
	if err := h.Get(p, result); err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}

	page := util.PageFromRequest(r)
	dataResponse := DataResponse{
		Path: result.Dataset.BodyPath,
		Data: json.RawMessage(result.Bytes),
	}
	if err := util.WritePageResponse(w, dataResponse, r, page); err != nil {
		log.Infof("error writing response: %s", err.Error())
	}
}

func (h DatasetHandlers) publishHandler(w http.ResponseWriter, r *http.Request, publish bool) {
	ref, err := DatasetRefFromPath(r.URL.Path[len("/publish"):])
	if err != nil {
		util.WriteErrResponse(w, http.StatusBadRequest, err)
		return
	}

	ref.Published = publish
	p := &lib.SetPublishStatusParams{
		Ref:               &ref,
		UpdateRegistry:    r.FormValue("no_registry") != "true",
		UpdateRegistryPin: r.FormValue("no_pin") != "true",
	}
	var ok bool
	if err := h.DatasetRequests.SetPublishStatus(p, &ok); err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	util.WriteResponse(w, ref)
}

func (h DatasetHandlers) updateHandler(w http.ResponseWriter, r *http.Request) {
	ref, err := DatasetRefFromPath(r.URL.Path[len("/update"):])
	if err != nil {
		util.WriteErrResponse(w, http.StatusBadRequest, err)
		return
	}

	p := &lib.UpdateParams{
		Ref:        ref.String(),
		Title:      r.FormValue("title"),
		Message:    r.FormValue("message"),
		DryRun:     r.FormValue("dry_run") == "true",
		ReturnBody: false,
	}

	if r.FormValue("secrets") != "" {
		p.Secrets = map[string]string{}
		if err := json.Unmarshal([]byte(r.FormValue("secrets")), &p.Secrets); err != nil {
			util.WriteErrResponse(w, http.StatusBadRequest, fmt.Errorf("parsing secrets: %s", err))
			return
		}
	}

	res := &repo.DatasetRef{}
	if err := h.DatasetRequests.Update(p, res); err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	util.WriteResponse(w, ref)
}

func (h DatasetHandlers) unpackHandler(w http.ResponseWriter, r *http.Request, postData []byte) {
	contents, err := dsutil.UnzipGetContents(postData)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	data, err := json.Marshal(contents)
	if err != nil {
		util.WriteErrResponse(w, http.StatusInternalServerError, err)
		return
	}
	util.WriteResponse(w, json.RawMessage(data))
}
