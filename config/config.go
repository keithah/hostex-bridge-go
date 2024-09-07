package config

import (
	"io/ioutil"
	"time"

	"gopkg.in/yaml.v2"
	"maunium.net/go/mautrix/id"
)

type Config struct {
	Homeserver struct {
		Address string `yaml:"address"`
		Domain  string `yaml:"domain"`
	} `yaml:"homeserver"`

	User struct {
		UserID string `yaml:"user_id"`
	} `yaml:"user"`

	Hostex struct {
		APIURL string `yaml:"api_url"`
		Token  string `yaml:"token"`
	} `yaml:"hostex"`

	Appservice struct {
		URL     string `yaml:"url"`
		ASToken string `yaml:"as_token"`
	} `yaml:"appservice"`

	Admin struct {
		UserID id.UserID `yaml:"user_id"`
	} `yaml:"admin"`

	Bridge struct {
		UserPrefix        string `yaml:"user_prefix"`
		UsernameTemplate  string `yaml:"username_template"`
		DisplaynameFormat string `yaml:"displayname_format"`
	} `yaml:"bridge"`

	Timezone            string        `yaml:"timezone"`
	PollInterval        time.Duration `yaml:"poll_interval"`
	PersonalSpaceEnable bool          `yaml:"personal_filtering_spaces"`

	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
}

func Load(path string) (*Config, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Timezone == "" {
		cfg.Timezone = "America/Los_Angeles"
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 10 * time.Second
	}

	return &cfg, nil
}
