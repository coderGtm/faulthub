package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"faulthub/internal/config"
	"faulthub/internal/ingest"
	"faulthub/internal/keygen"
	"faulthub/internal/store"
)

type seedTrace struct {
	trace   string
	hash    string
	weight  int
	comment string
	email   string
}

var seedTraces = []seedTrace{
	{
		trace: "java.lang.NullPointerException: Attempt to invoke virtual method on a null object reference\n" +
			"\tat com.example.app.MainActivity.onResume(MainActivity.kt:88)\n" +
			"\tat android.app.Activity.performResume(Activity.java:8091)",
		hash:   "seed-npe-main",
		weight: 40,
	},
	{
		trace: "java.lang.OutOfMemoryError: Failed to allocate a 24 byte allocation with 0 free bytes\n" +
			"\tat com.example.app.ImageCache.put(ImageCache.kt:41)\n" +
			"\tat com.example.app.FeedAdapter.onBind(FeedAdapter.kt:112)",
		hash:    "seed-oom-cache",
		weight:  20,
		comment: "froze while scrolling and then died",
		email:   "tester@example.com",
	},
	{
		trace: "android.app.RemoteServiceException$ForegroundServiceDidNotStartInTimeException: Context.startForegroundService() did not then call Service.startForeground()\n" +
			"\tat android.app.ActivityThread.handleServiceArgs(ActivityThread.java:4822)",
		hash:   "seed-anr-fgs",
		weight: 15,
	},
	{
		trace: "java.lang.SecurityException: Permission Denial: starting Intent from null (pid=1234) requires android.permission.ACCESS_FINE_LOCATION\n" +
			"\tat android.os.Parcel.createExceptionOrNull(Parcel.java:3069)",
		hash:   "seed-sec-loc",
		weight: 15,
	},
	{
		trace:  "",
		hash:   ingest.NoTraceHash,
		weight: 10,
	},
}

var seedVersions = [][2]string{{"2.4.1", "241"}, {"2.4.0", "240"}, {"2.3.9", "239"}}
var seedAndroids = []string{"14", "15", "13"}
var seedDevices = [][2]string{
	{"Google", "Pixel 8"}, {"Google", "Pixel 7"}, {"Samsung", "Galaxy S24"},
	{"OnePlus", "OnePlus 12"}, {"Xiaomi", "Redmi Note 13"},
}

func runSeed() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(cfg.DataDir, "faulthub.db"))
	if err != nil {
		return err
	}
	defer st.Close()
	ctx := context.Background()

	existing, err := st.ListApps(ctx)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, a := range existing {
		names[a.Name] = true
	}

	for _, appName := range []string{"Demo App", "Demo Shop"} {
		if names[appName] {
			fmt.Printf("%s already exists, skipping\n", appName)
			continue
		}
		key, hash, err := keygen.NewAPIKey()
		if err != nil {
			return err
		}
		pkg := "com.example.demoapp"
		if appName == "Demo Shop" {
			pkg = "com.example.demoshop"
		}
		app, err := st.CreateApp(ctx, appName, hash)
		if err != nil {
			return err
		}
		n := seedApp(ctx, st, app.ID, pkg)
		fmt.Printf("created %q: %d reports, api key %s\n", appName, n, key)
	}
	return nil
}

func seedApp(ctx context.Context, st *store.Store, appID int64, pkg string) int {
	rng := rand.New(rand.NewPCG(42, uint64(appID)))
	now := time.Now().UTC()
	n := 0
	for i := 0; i < 90; i++ {
		t := pickTrace(rng)
		day := weightedDay(rng)
		at := now.AddDate(0, 0, -day).Truncate(24 * time.Hour).
			Add(time.Duration(rng.IntN(24)) * time.Hour).
			Add(time.Duration(rng.IntN(60)) * time.Minute)
		ver := seedVersions[rng.IntN(len(seedVersions))]
		dev := seedDevices[rng.IntN(len(seedDevices))]
		inst := fmt.Sprintf("seed-inst-%d-%d", appID, rng.IntN(24))
		id := fmt.Sprintf("seed-%d-%04d", appID, i)
		rep := &store.Report{
			ID:               id,
			AppID:            appID,
			InstallationID:   inst,
			PackageName:      pkg,
			AppVersionCode:   ver[1],
			AppVersionName:   ver[0],
			AndroidVersion:   seedAndroids[rng.IntN(len(seedAndroids))],
			Brand:            dev[0],
			PhoneModel:       dev[1],
			Build:            "AP2A.240905.003",
			StackTrace:       t.trace,
			StackTraceHash:   t.hash,
			Title:            ingest.TitleFromTrace(t.trace, pkg),
			UserComment:      t.comment,
			UserEmail:        t.email,
			UserAppStartDate: at.Add(-time.Hour).Format(time.RFC3339),
			UserCrashDate:    at.Format(time.RFC3339),
			ReceivedAt:       at,
			Raw:              `{"REPORT_ID":"` + id + `"}`,
		}
		if _, err := st.IngestReport(ctx, rep); err != nil {
			continue
		}
		n++
	}
	return n
}

func pickTrace(rng *rand.Rand) seedTrace {
	total := 0
	for _, t := range seedTraces {
		total += t.weight
	}
	roll := rng.IntN(total)
	for _, t := range seedTraces {
		roll -= t.weight
		if roll < 0 {
			return t
		}
	}
	return seedTraces[0]
}

func weightedDay(rng *rand.Rand) int {
	day := rng.IntN(30)
	if rng.IntN(100) < 60 {
		day = rng.IntN(7)
	}
	return day
}
