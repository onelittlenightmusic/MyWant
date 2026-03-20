package types

import (
	"encoding/json"
	"fmt"
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
	o.StoreState("osm_downloaded", false)
	o.StoreState("graph_built", false)
	o.StoreState("server_running", false)
	// If no GTFS data is configured, mark gtfs_downloaded=true so OPA
	// treats it as already satisfied and does not dispatch download_gtfs.
	if o.GetStringParam("gtfs_url", "") == "" && o.GetStringParam("gtfs_feeds", "") == "" {
		o.StoreState("gtfs_downloaded", true)
	} else {
		o.StoreState("gtfs_downloaded", false)
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

	// Add GTFS download step when one or more feeds are configured.
	// Multiple feeds are downloaded sequentially inside a single container.
	if feeds := o.resolveGTFSFeeds(); len(feeds) > 0 {
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
	}

	b, _ := json.Marshal(dm)
	return string(b)
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
		"osm_downloaded":  GetState[bool](&o.Want, "osm_downloaded", false),
		"gtfs_downloaded": GetState[bool](&o.Want, "gtfs_downloaded", false),
		"graph_built":     GetState[bool](&o.Want, "graph_built", false),
		"server_running":  GetState[bool](&o.Want, "server_running", false),
	})
}
