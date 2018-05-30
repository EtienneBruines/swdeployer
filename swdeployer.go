package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/persistent-cookiejar"
	"github.com/mcuadros/go-version"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"gopkg.in/ini.v1"
)

const shopwareAPI = "https://api.shopware.com/"

func expandIfNeeded(s string) string {
	if s[0] == '~' {
		u, err := user.Current()
		if err != nil {
			return s
		}

		return filepath.Join(u.HomeDir, s[1:])
	}

	return s
}

func loadConfig(path string) (*ini.Section, error) {
	f, err := os.Open(expandIfNeeded(path))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to open config file %s", path)
	}

	i, err := ini.Load(f)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to parse ini file %s", path)
	}

	section, err := i.GetSection("")
	if err != nil {
		return nil, errors.Wrap(err, "unable to find default (unnamed) section")
	}

	return section, nil
}

func (sw *ShopwareClient) printPluginInfo(pluginID int) error {
	if pluginID == 0 {
		return errors.New("plugin_id not found")
	}
	sw.pluginID = pluginID

	req, err := http.NewRequest("GET", shopwareAPI+"plugins/"+strconv.Itoa(pluginID), nil)
	if err != nil {
		return errors.Wrap(err, "unable to create plugin info request")
	}

	req.Header.Set("X-Shopware-Token", sw.token)
	resp, err := sw.c.Do(req)
	if err != nil {
		return errors.Wrap(err, "unable to retrieve plugin info")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status code when retrieving plugin info: %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)

	var pluginInfo = PluginInfoResult{}
	err = dec.Decode(&pluginInfo)
	if err != nil {
		return errors.Wrap(err, "unable to unmarshal json plugin info response")
	}

	fmt.Printf("Name:\t\t %s (%s)\n", pluginInfo.Name, pluginInfo.ActivationStatus.Name)
	fmt.Println("Addons:\t\t", pluginInfo.Addons)
	fmt.Println("Changed at:\t", pluginInfo.LatestBinary.LastChangeDate)
	fmt.Println("Version:\t", pluginInfo.LatestBinary.Version, "(current)")
	fmt.Println()

	return nil
}

func (sw *ShopwareClient) printNewData() error {
	f, err := os.Open("plugin.xml")
	if err != nil {
		return errors.Wrap(err, "unable to open plugin.xml")
	}

	dec := xml.NewDecoder(f)
	err = dec.Decode(&sw.pluginInfo)
	if err != nil {
		return errors.Wrap(err, "unable to decode plugin.xml")
	}

	fmt.Println("Version:\t", sw.pluginInfo.Version, "(new)")
	if len(sw.pluginInfo.Changelog) > 0 {
		last := sw.pluginInfo.Changelog[len(sw.pluginInfo.Changelog)-1]
		for _, change := range last.Changes {
			if change.Lang == "" {
				change.Lang = "en"
			}

			fmt.Printf("Log %s:\t [%s] %s\n", last.Version, change.Lang, change.Value)
		}
	} else {
		fmt.Println("No changelog was found")
	}

	fmt.Println()

	return nil
}

type CompatibleSoftwareVersion struct {
	Checked    *bool  `json:"checked,omitempty"`
	ID         int    `json:"id"`
	Major      string `json:"major"`
	Name       string `json:"name"`
	Parent     int    `json:"parent"`
	Selectable bool   `json:"selectable"`
}

type Changelog struct {
	ID     int `json:"id"`
	Locale struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"locale"`
	Text string `json:"text"`
}

type BinaryUploadResponse struct {
	Archives []struct {
		ID                   int     `json:"id"`
		IonCubeEncrypted     bool    `json:"ioncubeEncrypted"`
		RemoteLink           string  `json:"remoteLink"`
		ShopwareMajorVersion *string `json:"shopwareMajorVersion"`
	} `json:"archives"`
	Assessment                 bool                        `json:"assessment"`
	Changelogs                 []Changelog                 `json:"changelogs"`
	CompatibleSoftwareVersions []CompatibleSoftwareVersion `json:"compatibleSoftwareVersions"`
	CreationDate               string                      `json:"creationDate"`
	ID                         int                         `json:"id"`
	IonCubeEncrypted           bool                        `json:"ionCubeEncrypted"`
	LastChangeDate             string                      `json:"lastChangeDate"`
	LicenseCheckRequired       bool                        `json:"licenseCheckRequired"`
	Name                       string                      `json:"name"`
	RemoteLink                 string                      `json:"remoteLink"`
	Status                     struct {
		Description string `json:"description"`
		ID          int    `json:"id"`
		Name        string `json:"name"`
	} `json:"status"`
	Version string `json:"version"`
}

const sw5 = "Shopware 5"

var FalseVariable = false

var compatibleSoftwareVersions = map[string]CompatibleSoftwareVersion{
    "5.4.4": {
        Checked:    &FalseVariable,
        ID:         112,
        Major:      sw5,
        Name:E      "5.4.4",
        Parent:     105,
        Selectable: true,
    },
	"5.4.3": {
		Checked:    &FalseVariable,
		ID:         109,
		Major:      sw5,
		Name:       "5.4.3",
		Parent:     105,
		Selectable: true,
	},
	"5.4.2": {
		Checked:    &FalseVariable,
		ID:         108,
		Major:      sw5,
		Name:       "5.4.2",
		Parent:     105,
		Selectable: true,
	},
	"5.4.1": {
		Checked:    &FalseVariable,
		ID:         107,
		Major:      sw5,
		Name:       "5.4.1",
		Parent:     105,
		Selectable: true,
	},
	"5.4.0": {
		Checked:    &FalseVariable,
		ID:         106,
		Major:      sw5,
		Name:       "5.4.0",
		Parent:     105,
		Selectable: true,
	},
	"5.3.7": {
		Checked:    &FalseVariable,
		ID:         104,
		Major:      sw5,
		Name:       "5.3.7",
		Parent:     94,
		Selectable: true,
	},
	"5.3.6": {
		Checked:    &FalseVariable,
		ID:         103,
		Major:      sw5,
		Name:       "5.3.6",
		Parent:     94,
		Selectable: true,
	},
	"5.3.5": {
		Checked:    &FalseVariable,
		ID:         102,
		Major:      sw5,
		Name:       "5.3.5",
		Parent:     94,
		Selectable: true,
	},
	"5.3.4": {
		Checked:    &FalseVariable,
		ID:         101,
		Major:      sw5,
		Name:       "5.3.4",
		Parent:     94,
		Selectable: true,
	},
	"5.3.3": {
		Checked:    &FalseVariable,
		ID:         100,
		Major:      sw5,
		Name:       "5.3.3",
		Parent:     94,
		Selectable: true,
	},
	"5.3.2": {
		Checked:    &FalseVariable,
		ID:         99,
		Major:      sw5,
		Name:       "5.3.2",
		Parent:     94,
		Selectable: true,
	},
	"5.3.1": {
		Checked:    &FalseVariable,
		ID:         98,
		Major:      sw5,
		Name:       "5.3.1",
		Parent:     94,
		Selectable: true,
	},
	"5.3.0": {
		Checked:    &FalseVariable,
		ID:         93,
		Major:      sw5,
		Name:       "5.3.0",
		Parent:     94,
		Selectable: true,
	},
	"5.2.27": {
		Checked:    &FalseVariable,
		ID:         97,
		Major:      sw5,
		Name:       "5.2.27",
		Parent:     66,
		Selectable: true,
	},
	"5.2.26": {
		Checked:    &FalseVariable,
		ID:         96,
		Major:      sw5,
		Name:       "5.2.26",
		Parent:     66,
		Selectable: true,
	},
	"5.2.25": {
		Checked:    &FalseVariable,
		ID:         95,
		Major:      sw5,
		Name:       "5.2.25",
		Parent:     66,
		Selectable: true,
	},
	"5.2.24": {
		Checked:    &FalseVariable,
		ID:         92,
		Major:      sw5,
		Name:       "5.2.24",
		Parent:     66,
		Selectable: true,
	},
	"5.2.23": {
		Checked:    &FalseVariable,
		ID:         91,
		Major:      sw5,
		Name:       "5.2.23",
		Parent:     66,
		Selectable: true,
	},
	"5.2.22": {
		Checked:    &FalseVariable,
		ID:         90,
		Major:      sw5,
		Name:       "5.2.22",
		Parent:     66,
		Selectable: true,
	},
	"5.2.21": {
		Checked:    &FalseVariable,
		ID:         89,
		Major:      sw5,
		Name:       "5.2.21",
		Parent:     66,
		Selectable: true,
	},
	"5.2.20": {
		Checked:    &FalseVariable,
		ID:         88,
		Major:      sw5,
		Name:       "5.2.20",
		Parent:     66,
		Selectable: true,
	},
	"5.2.19": {
		Checked:    &FalseVariable,
		ID:         87,
		Major:      sw5,
		Name:       "5.2.19",
		Parent:     66,
		Selectable: true,
	},
	"5.2.18": {
		Checked:    &FalseVariable,
		ID:         86,
		Major:      sw5,
		Name:       "5.2.18",
		Parent:     66,
		Selectable: true,
	},
	"5.2.17": {
		Checked:    &FalseVariable,
		ID:         85,
		Major:      sw5,
		Name:       "5.2.17",
		Parent:     66,
		Selectable: true,
	},
	"5.2.16": {
		Checked:    &FalseVariable,
		ID:         84,
		Major:      sw5,
		Name:       "5.2.16",
		Parent:     66,
		Selectable: true,
	},
	"5.2.15": {
		Checked:    &FalseVariable,
		ID:         83,
		Major:      sw5,
		Name:       "5.2.15",
		Parent:     66,
		Selectable: true,
	},
	"5.2.14": {
		Checked:    &FalseVariable,
		ID:         82,
		Major:      sw5,
		Name:       "5.2.14",
		Parent:     66,
		Selectable: true,
	},
	"5.2.13": {
		Checked:    &FalseVariable,
		ID:         81,
		Major:      sw5,
		Name:       "5.2.13",
		Parent:     66,
		Selectable: true,
	},
	"5.2.12": {
		Checked:    &FalseVariable,
		ID:         80,
		Major:      sw5,
		Name:       "5.2.12",
		Parent:     66,
		Selectable: true,
	},
	"5.2.11": {
		Checked:    &FalseVariable,
		ID:         79,
		Major:      sw5,
		Name:       "5.2.11",
		Parent:     66,
		Selectable: true,
	},
	"5.2.10": {
		Checked:    &FalseVariable,
		ID:         78,
		Major:      sw5,
		Name:       "5.2.10",
		Parent:     66,
		Selectable: true,
	},
	"5.2.9": {
		Checked:    &FalseVariable,
		ID:         77,
		Major:      sw5,
		Name:       "5.2.9",
		Parent:     66,
		Selectable: true,
	},
	"5.2.8": {
		Checked:    &FalseVariable,
		ID:         76,
		Major:      sw5,
		Name:       "5.2.8",
		Parent:     66,
		Selectable: true,
	},
	"5.2.7": {
		Checked:    &FalseVariable,
		ID:         75,
		Major:      sw5,
		Name:       "5.2.7",
		Parent:     66,
		Selectable: true,
	},
	"5.2.6": {
		Checked:    &FalseVariable,
		ID:         74,
		Major:      sw5,
		Name:       "5.2.6",
		Parent:     66,
		Selectable: true,
	},
	"5.2.5": {
		Checked:    &FalseVariable,
		ID:         73,
		Major:      sw5,
		Name:       "5.2.5",
		Parent:     66,
		Selectable: true,
	},
	"5.2.4": {
		Checked:    &FalseVariable,
		ID:         72,
		Major:      sw5,
		Name:       "5.2.4",
		Parent:     66,
		Selectable: true,
	},
	"5.2.3": {
		Checked:    &FalseVariable,
		ID:         71,
		Major:      sw5,
		Name:       "5.2.3",
		Parent:     66,
		Selectable: true,
	},
	"5.2.2": {
		Checked:    &FalseVariable,
		ID:         70,
		Major:      sw5,
		Name:       "5.2.2",
		Parent:     66,
		Selectable: true,
	},
	"5.2.1": {
		Checked:    &FalseVariable,
		ID:         69,
		Major:      sw5,
		Name:       "5.2.1",
		Parent:     66,
		Selectable: true,
	},
	"5.2.0": {
		Checked:    &FalseVariable,
		ID:         67,
		Major:      sw5,
		Name:       "5.2.0",
		Parent:     66,
		Selectable: true,
	},
}

func compatibleVersions(from, to string) (versions []CompatibleSoftwareVersion) {
	if to == "" {
		to = "5.4.4"
	}

	if from == "" {
		return
	}

	for ver, csv := range compatibleSoftwareVersions {
		if version.Compare(ver, from, ">=") && version.Compare(ver, to, "<=") {
			versions = append(versions, csv)
		}
	}

	return
}

func (b *BinaryUploadResponse) SetChangelog(locale, text string) {
	for i, ch := range b.Changelogs {
		if ch.Locale.Name == locale {
			ch.Text = text
		}
		b.Changelogs[i] = ch
	}
}

func (sw *ShopwareClient) update() error {
	fmt.Print("Are you sure you want to deploy the new version to Shopware? (y/N) ")

	var response string
	fmt.Scanf("%s", &response)

	if !strings.EqualFold(strings.TrimSpace(response), "y") {
		return errors.New("aborted by user")
	}

	// Step -1: make sure directory is clean
	clean := exec.Command("git", "diff-index", "--quiet", "HEAD", "--")
	err := clean.Run()
	if err != nil {
		return errors.New("your git directory is not clean. Please commit your changes and try again")
	}

	// Step 0: prepare the binary
	prefix, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "unable to figure out current working directory")
	}
	prefix = filepath.Base(prefix)

	filename := filepath.Join(os.TempDir(), strconv.Itoa(rand.Int())+".zip")
	cmd := exec.Command("git", "archive", "-o", filename, "-9", "--prefix", prefix+"/", "HEAD")
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, "unable to prepare zip file")
	}
	defer os.Remove(filename)

	// Step 1: upload binary
	respBody, err := sw.uploadFile(shopwareAPI+"plugins/"+strconv.Itoa(sw.pluginID)+"/binaries", filename, prefix+".zip")
	if err != nil {
		return errors.Wrap(err, "unable to upload plugin zip")
	}

	var responses []BinaryUploadResponse
	err = json.Unmarshal(respBody, &responses)
	if err != nil {
		return errors.Wrap(err, "unable to unmarshal upload result")
	}
	if len(responses) != 1 {
		return fmt.Errorf("not the correct amount of responses to binary upload: %d", len(responses))
	}
	binaryDetails := responses[0]
	fmt.Println("Binary uploaded")

	// Then we get a big object
	// Step 2: verify upload
	// GET to https://api.shopware.com/plugins/5998/binaries/23660
	// (not sure if required, because object looks identical to upload result)

	// status.name == "waitingforcodereview"

	// Step 3: set the metadata for this version
	binaryDetails.SetChangelog("de_DE", sw.lastChangelogByLocale("de"))
	binaryDetails.SetChangelog("en_GB", sw.lastChangelogByLocale("en"))
	binaryDetails.CompatibleSoftwareVersions = compatibleVersions(sw.pluginInfo.Compatibility.MinVersion, sw.pluginInfo.Compatibility.MaxVersion)
	binaryDetails.LicenseCheckRequired = true
	binaryDetails.Version = sw.pluginInfo.Version
	err = sw.put(shopwareAPI+"plugins/"+strconv.Itoa(sw.pluginID)+"/binaries/"+strconv.Itoa(binaryDetails.ID), binaryDetails)
	if err != nil {
		return errors.Wrap(err, "unable to upload changelog data")
	}
	fmt.Println("Meta-data uploaded")

	// Step 4a:
	fmt.Println()
	fmt.Print("Would you like to request a code-review? (y/N) ")
	// TODO: Scan for the reply

	// Step 4b: POST request to /plugins/5411/reviews
	// with empty json payload (optional?)
	// to request code review

	// Step 5: keep asking https://api.shopware.com/plugins/5411/binaries/23730/checkresults and check for the review

	// Step 6: compare type.name for "automaticcodereviewsucceeded", or perhaps "requested"

	// Then we might want to verify the file uploaded.
	// GET to https://api.shopware.com/plugins/5998/binaries/23660/file?token=f02464d52f2782443447420c7bbafeed5a02e5bd73da91.19108458&shopwareMajorVersion=52
	// (we could verify plugin.xml for version number, for example)

	// Optionally prompt to restart this circus
	// if so: we probably want to delete that newly-uploaded version
	// or we have to re-use it and upload the binary again to this version

	return nil
}

func (sw *ShopwareClient) put(url string, object interface{}) error {
	b, err := json.Marshal(object)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", url, bytes.NewReader(b))
	if err != nil {
		return err
	}

	req.Header.Set("X-Shopware-Token", sw.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := sw.c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("bad status code: %d - %s", resp.StatusCode, string(body))
	}

	return nil
}

func (sw *ShopwareClient) uploadFile(url, file, name string) ([]byte, error) {
	// Prepare a form that you will submit to that URL.
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	// Add your image file
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fw, err := w.CreateFormFile("name", file)
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(fw, f); err != nil {
		return nil, err
	}
	// Don't forget to close the multipart writer.
	// If you don't close it, your request will be missing the terminating boundary.
	w.Close()

	// Now that you have a form, you can submit it to your handler.
	req, err := http.NewRequest("POST", url, &b)
	if err != nil {
		return nil, err
	}
	// Don't forget to set the content type, this will contain the boundary.
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Shopware-Token", sw.token)

	// Submit the request
	res, err := sw.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	// Check the response
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

type PluginInfoResult struct {
	Name             string `json:"name"`
	LastChange       string `json:"lastChange"`
	ActivationStatus struct {
		Name string `json:"name"`
	} `json:"activationStatus"`
	Addons         Addons `json:"addons"`
	ApprovalStatus struct {
		Name string `json:"name"`
	} `json:"approvalStatus"`
	LatestBinary struct {
		LastChangeDate string `json:"lastChangeDate"`
		Version        string `json:"version"`
	} `json:"latestBinary"`
}

type Addons []struct {
	Name string `json:"name"`
}

func (a Addons) String() string {
	var str []string
	for _, add := range a {
		str = append(str, add.Name)
	}
	return strings.Join(str, ", ")
}

type ShopwareClient struct {
	c        *http.Client
	cfg      *ini.Section
	username string
	password string
	token    string
	pluginID int

	pluginInfo struct {
		Version   string `xml:"version"`
		Changelog []struct {
			Version string `xml:"version,attr"`
			Changes []struct {
				Lang  string `xml:"lang,attr"`
				Value string `xml:",innerxml"`
			} `xml:"changes"`
		} `xml:"changelog"`
		Compatibility struct {
			MinVersion string `xml:"minVersion,attr"`
			MaxVersion string `xml:"maxVersion,attr"`
		} `xml:"compatibility"`
	}
}

func (sw *ShopwareClient) lastChangelogByLocale(locale string) string {
	if len(sw.pluginInfo.Changelog) == 0 {
		return ""
	}

	last := sw.pluginInfo.Changelog[len(sw.pluginInfo.Changelog)-1]
	for _, ch := range last.Changes {
		if ch.Lang == locale {
			return ch.Value
		}
	}

	if len(last.Changes) == 0 {
		return ""
	}

	return last.Changes[0].Value
}

func newClient(c *cli.Context, cfg *ini.Section) (*ShopwareClient, error) {
	var err error

	sw := &ShopwareClient{
		c:   &http.Client{},
		cfg: cfg,
	}
	sw.c.Jar, err = cookiejar.New(&cookiejar.Options{
		Filename: c.String("jar"),
	})
	if err != nil {
		return nil, errors.Wrap(err, "unable to load cookie jar")
	}

	b, err := ioutil.ReadFile(expandIfNeeded(c.String("credentials")))
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read credentials file: %s", cfg.Key("credentials").String())
	}

	lines := bytes.Split(b, []byte("\n"))
	for i, line := range lines {
		switch i {
		case 0:
			sw.username = string(bytes.TrimSpace(line))
		case 1:
			sw.password = string(bytes.TrimSpace(line))
		}
	}

	return sw, nil
}

func (sw *ShopwareClient) login() error {
	body, err := json.Marshal(struct {
		Username string `json:"shopwareId"`
		Password string `json:"password"`
	}{sw.username, sw.password})
	if err != nil {
		return errors.Wrap(err, "unable to prepare login request")
	}

	resp, err := sw.c.Post(shopwareAPI+"accesstokens", "application/json", bytes.NewReader(body))
	if err != nil {
		return errors.Wrap(err, "bad result when logging in")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("unexpected status code when logging in: %d", resp.StatusCode)
	}

	var data = struct {
		Token string `json:"token"`
	}{}
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&data)
	if err != nil {
		return errors.Wrap(err, "unable to decode login json response")
	}

	sw.token = data.Token

	return nil
}

func logic(c *cli.Context) error {
	cfg, err := loadConfig(c.String("config"))
	if err != nil {
		return err
	}

	client, err := newClient(c, cfg)
	if err != nil {
		return err
	}

	err = client.login()
	if err != nil {
		return err
	}

	err = client.printPluginInfo(cfg.Key("plugin_id").MustInt())
	if err != nil {
		return err
	}

	err = client.printNewData()
	if err != nil {
		return err
	}

	err = client.update()
	if err != nil {
		return err
	}

	return nil
}

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config",
			Value: ".shopware-deploy.ini",
			Usage: "Location of the .shopware-deploy.ini file for this plugin",
		},
		cli.StringFlag{
			Name:  "jar",
			Value: "~/.cache/shopware-deploy",
			Usage: "Location of the shopware-deploy cookie jar",
		},
		cli.StringFlag{
			Name:  "credentials",
			Value: "~/.config/shopware-deploy",
			Usage: "Location of the shopware-deploy global settings file",
		},
	}
	app.Name = "Shopware Deployer (unofficial)"
	app.Authors = []cli.Author{
		{
			Name:  "Etienne Bruines",
			Email: "etienne.bruines@webcustoms.de",
		},
	}
	app.Version = "0.0.5.4.3"
	app.Action = func(c *cli.Context) error {
		err := logic(c)
		if err != nil {
			fmt.Println("Error:", err.Error())
		}
		return err
	}

	app.Run(os.Args)
}
