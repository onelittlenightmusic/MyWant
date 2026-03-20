package types

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "mywant/engine/core"
)

func init() {
	RegisterWantImplementation[OtpSetupWant, OtpSetupLocals]("otp_setup")
}

type OtpSetupLocals struct{}

// OtpSetupWant is an OPA-driven coordinator that provisions a full OpenTripPlanner stack.
//
// Flow (controlled by otp_setup.rego):
//
//	download_osm ┐
//	             ├→ build_graph → start_server
//	download_gtfs┘
//
// OSM and GTFS downloads run in parallel; build_graph starts once both complete.
// Each step is a docker_run child Want dispatched by DispatchThinker.
// Flags (osm_downloaded, gtfs_downloaded, graph_built, server_running) are set via
// direction_map "sets" when each child achieves, and synced into the OPA current snapshot in Progress().
type OtpSetupWant struct {
	Want
}

func (o *OtpSetupWant) GetLocals() *OtpSetupLocals {
	return CheckLocalsInitialized[OtpSetupLocals](&o.Want)
}

func (o *OtpSetupWant) Initialize() {
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")

	// Idempotent checks: skip steps whose outputs already exist on disk.
	osmExists := fileExists(filepath.Join(dataDir, o.GetStringParam("osm_filename", "map.osm.pbf")))
	graphExists := fileExists(filepath.Join(dataDir, "graph.obj"))

	o.StoreState("osm_downloaded", osmExists)
	o.StoreState("graph_built", graphExists)
	o.StoreState("server_running", false)

	// If no GTFS data is configured, mark gtfs_downloaded and build_config_written=true
	// so OPA treats them as already satisfied.
	hasGTFS := o.GetStringParam("gtfs_url", "") != "" || o.GetStringParam("gtfs_feeds", "") != ""
	if hasGTFS {
		// Check each GTFS file exists; only mark downloaded if all are present.
		allGTFSExist := true
		for _, f := range o.resolveGTFSFeeds() {
			if !fileExists(filepath.Join(dataDir, f.Filename)) {
				allGTFSExist = false
				break
			}
		}
		o.StoreState("gtfs_downloaded", allGTFSExist)
		// build-config.json is always rewritten when GTFS is present and graph needs building.
		o.StoreState("build_config_written", graphExists || fileExists(filepath.Join(dataDir, "build-config.json")))
	} else {
		o.StoreState("gtfs_downloaded", true)
		o.StoreState("build_config_written", true)
	}

	if osmExists {
		o.StoreLog("[OTP] OSM file already exists — skipping download")
	}
	if graphExists {
		o.StoreLog("[OTP] graph.obj already exists — skipping build")
	}

	// Promote OPA planner params → state so ThinkingAgent can read via GetCurrent.
	o.SetCurrent("opa_llm_planner_command", o.GetStringParam("opa_llm_planner_command", "opa-llm-planner"))
	o.SetCurrent("policy_dir", o.GetStringParam("policy_dir", "yaml/policies"))
	o.SetCurrent("use_llm", o.GetBoolParam("use_llm", false))

	// Set static goal so OPA thinker has a non-empty goal to evaluate against.
	o.SetGoal("goal", map[string]any{"setup_complete": true})

	// Build direction_map from individual params if not already provided.
	existing := GetCurrent(o, "direction_map", "")
	if existing == "" || existing == "{}" {
		dm := o.buildDirectionMap()
		o.SetCurrent("direction_map", dm)
		o.Spec.Params["direction_map"] = dm
	}

	// Start DispatchThinker to realize child Wants from directions produced by OPA thinker.
	dispatchID := DispatchThinkerName + "-" + o.Metadata.ID
	if _, running := o.GetBackgroundAgent(dispatchID); !running {
		agent := NewDispatchThinker(dispatchID)
		if err := o.AddBackgroundAgent(agent); err != nil {
			o.StoreLog("[OTP] ERROR: Failed to start DispatchThinker: %v", err)
		}
	}

	o.StoreLog("[OTP] Initialized — setup flags reset")
}

// buildDirectionMap constructs the direction_map JSON string from individual params.
// gtfsFeed holds a single GTFS feed URL and filename for download.
type gtfsFeed struct {
	URL      string `json:"url"`
	Filename string `json:"filename"`
}

// resolveGTFSFeeds returns the list of GTFS feeds to download.
// Priority: gtfs_feeds (JSON array) > legacy gtfs_url/gtfs_filename.
func (o *OtpSetupWant) resolveGTFSFeeds() []gtfsFeed {
	feedsJSON := o.GetStringParam("gtfs_feeds", "")
	if feedsJSON != "" {
		var feeds []gtfsFeed
		if err := json.Unmarshal([]byte(feedsJSON), &feeds); err == nil && len(feeds) > 0 {
			return feeds
		}
	}
	// Legacy single-feed params.
	singleURL := o.GetStringParam("gtfs_url", "")
	if singleURL != "" {
		return []gtfsFeed{{URL: singleURL, Filename: o.GetStringParam("gtfs_filename", "gtfs.zip")}}
	}
	return nil
}

func (o *OtpSetupWant) buildDirectionMap() string {
	dataDir := o.GetStringParam("data_dir", "/tmp/otp-data")
	osmURL := o.GetStringParam("osm_url", "https://download.bbbike.org/osm/bbbike/Tokyo/Tokyo.osm.pbf")
	osmFilename := o.GetStringParam("osm_filename", "map.osm.pbf")
	buildMem := o.GetStringParam("build_memory", "-Xmx6G")
	serverMem := o.GetStringParam("server_memory", "-Xmx4G")
	serverPort := o.GetStringParam("server_port", "8080")

	volumeMount := fmt.Sprintf("[\"%s:/var/opentripplanner\"]", dataDir)
	downloadVolume := fmt.Sprintf("[\"%s:/data\"]", dataDir)
	ports := fmt.Sprintf("[\"%s:8080\"]", serverPort)
	healthURL := fmt.Sprintf("http://localhost:%s/", serverPort)

	dm := map[string]any{
		"download_osm": map[string]any{
			"type": "docker_run",
			"params": map[string]any{
				// alpine + wget: idempotent — skip download if file already exists
				"image":          "alpine:latest",
				"container_name": "otp-download-osm",
				"volumes":        downloadVolume,
				"command_args":   fmt.Sprintf("[\"sh\", \"-c\", \"[ -f /data/%s ] && echo 'OSM file already exists, skipping' || wget -O /data/%s '%s'\"]", osmFilename, osmFilename, osmURL),
				"wait_for_exit":  true,
			},
			"sets": map[string]any{"osm_downloaded": true},
		},
		"build_graph": map[string]any{
			"type": "docker_run",
			"params": map[string]any{
				"image":          "docker.io/opentripplanner/opentripplanner:2.6.0",
				"container_name": "otp-builder",
				"volumes":        volumeMount,
				"env":            fmt.Sprintf("{\"JAVA_OPTIONS\": \"%s\"}", buildMem),
				"command_args":   "[\"--build\", \"--save\"]",
				"wait_for_exit":  true,
			},
			"sets": map[string]any{"graph_built": true},
		},
		"start_server": map[string]any{
			"type": "docker_run",
			"params": map[string]any{
				"image":                    "docker.io/opentripplanner/opentripplanner:2.6.0",
				"container_name":           "otp-server",
				"ports":                    ports,
				"volumes":                  volumeMount,
				"env":                      fmt.Sprintf("{\"JAVA_OPTIONS\": \"%s\"}", serverMem),
				"command_args":             "[\"--load\", \"--serve\"]",
				"health_check_url":         healthURL,
				"health_check_interval":    "5s",
				"health_check_max_retries": 120,
			},
			"sets": map[string]any{"server_running": true},
		},
	}

	// Add GTFS download + build-config.json steps when one or more feeds are configured.
	if feeds := o.resolveGTFSFeeds(); len(feeds) > 0 {
		// download_gtfs: download all feeds sequentially, idempotent.
		var cmds []string
		for _, f := range feeds {
			cmds = append(cmds, fmt.Sprintf(
				"[ -f /data/%s ] && echo 'GTFS %s already exists, skipping' || wget -O /data/%s '%s'",
				f.Filename, f.Filename, f.Filename, f.URL,
			))
		}
		shellScript := strings.Join(cmds, " && ")
		dm["download_gtfs"] = map[string]any{
			"type": "docker_run",
			"params": map[string]any{
				"image":          "alpine:latest",
				"container_name": "otp-download-gtfs",
				"volumes":        downloadVolume,
				"command_args":   fmt.Sprintf("[\"sh\", \"-c\", \"%s\"]", strings.ReplaceAll(shellScript, "\"", "\\\"")),
				"wait_for_exit":  true,
			},
			"sets": map[string]any{"gtfs_downloaded": true},
		}

		// write_build_config: write build-config.json so OTP recognises the GTFS feeds.
		// OTP 2.x does not auto-detect GTFS zips; explicit transitFeeds config is required.
		// We base64-encode the JSON to avoid escaping issues when passing through shell.
		var feedEntries []string
		for _, f := range feeds {
			feedEntries = append(feedEntries, fmt.Sprintf(`{"type": "gtfs", "source": "%s"}`, f.Filename))
		}
		buildConfigJSON := fmt.Sprintf(`{"transitFeeds": [%s]}`, strings.Join(feedEntries, ", "))
		b64Config := base64Encode(buildConfigJSON)
		dm["write_build_config"] = map[string]any{
			"type": "docker_run",
			"params": map[string]any{
				"image":          "alpine:latest",
				"container_name": "otp-write-build-config",
				"volumes":        downloadVolume,
				"command_args":   fmt.Sprintf("[\"sh\", \"-c\", \"echo %s | base64 -d > /data/build-config.json && echo 'build-config.json written'\"]", b64Config),
				"wait_for_exit":  true,
			},
			"sets": map[string]any{"build_config_written": true},
		}
	}

	b, _ := json.Marshal(dm)
	return string(b)
}

func base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsAchieved returns true once the OTP server is up and healthy.
func (o *OtpSetupWant) IsAchieved() bool {
	return GetState[bool](&o.Want, "server_running", false)
}

func (o *OtpSetupWant) CalculateAchievingPercentage() float64 {
	osmDone := GetState[bool](&o.Want, "osm_downloaded", false)
	gtfsDone := GetState[bool](&o.Want, "gtfs_downloaded", false)
	hasGTFS := len(o.resolveGTFSFeeds()) > 0
	switch {
	case GetState[bool](&o.Want, "server_running", false):
		return 100
	case GetState[bool](&o.Want, "graph_built", false):
		return 75
	case osmDone && (!hasGTFS || gtfsDone):
		return 40
	case osmDone || gtfsDone:
		return 20
	default:
		return 5
	}
}

// Progress syncs completion flags into the OPA current snapshot so DispatchThinker
// can evaluate otp_setup.rego and dispatch the next step.
func (o *OtpSetupWant) Progress() {
	o.SetCurrent("achieving_percentage", o.CalculateAchievingPercentage())

	o.SetCurrent("current", map[string]any{
		"osm_downloaded":       GetState[bool](&o.Want, "osm_downloaded", false),
		"gtfs_downloaded":      GetState[bool](&o.Want, "gtfs_downloaded", false),
		"build_config_written": GetState[bool](&o.Want, "build_config_written", false),
		"graph_built":          GetState[bool](&o.Want, "graph_built", false),
		"server_running":       GetState[bool](&o.Want, "server_running", false),
	})
}
