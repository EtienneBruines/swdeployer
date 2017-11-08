package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"os/exec"

	"math/rand"

	"github.com/juju/persistent-cookiejar"
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

func (c *ShopwareClient) printNewData() error {
	f, err := os.Open("plugin.xml")
	if err != nil {
		return errors.Wrap(err, "unable to open plugin.xml")
	}

	var pluginInfo struct {
		Version   string `xml:"version"`
		Changelog []struct {
			Version string `xml:"version,attr"`
			Changes []struct {
				Lang  string `xml:"lang,attr"`
				Value string `xml:",innerxml"`
			} `xml:"changes"`
		} `xml:"changelog"`
	}

	dec := xml.NewDecoder(f)
	err = dec.Decode(&pluginInfo)
	if err != nil {
		return errors.Wrap(err, "unable to decode plugin.xml")
	}

	fmt.Println("Version:\t", pluginInfo.Version, "(new)")
	if len(pluginInfo.Changelog) > 0 {
		last := pluginInfo.Changelog[len(pluginInfo.Changelog)-1]
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

func (c *ShopwareClient) update() error {
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
	cmd := exec.Command("git", "archive", "-o", filename, "-9", "--prefix", prefix, "HEAD")
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		return errors.Wrap(err, "unable to prepare zip file")
	}
	os.Remove(filename) // fail silently

	// Step 1: upload binary
	// POST to https://api.shopware.com/plugins/5998/binaries
	// with multipart/form-data, name="file";

	// Then we get a big object
	// Step 2: verify upload
	// GET to https://api.shopware.com/plugins/5998/binaries/23660
	// (not sure if required, because object looks identical to upload result)

	// Step 3: set the metadata for this version
	// PUT to https://api.shopware.com/plugins/5998/binaries/23660

	// Important parts: compatibleSoftwareVersions, changelogs

	// Then we might want to verify the file uploaded.
	// GET to https://api.shopware.com/plugins/5998/binaries/23660/file?token=f02464d52f2782443447420c7bbafeed5a02e5bd73da91.19108458&shopwareMajorVersion=52
	// (we could verify plugin.xml for version number, for example)

	// Optionally prompt to restart this circus
	// if so: we probably want to delete that newly-uploaded version
	// or we have to re-use it and upload the binary again to this version

	return nil
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
	app.Version = "0.0.1"
	app.Action = func(c *cli.Context) error {
		err := logic(c)
		if err != nil {
			fmt.Println("Error:", err.Error())
		}
		return err
	}

	app.Run(os.Args)
}
