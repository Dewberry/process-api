// Package processes register processes from yaml specs
// and provide types and function to interact with these processes
package processes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type process struct {
	Info    Info      `yaml:"info"`
	Runtime Runtime   `yaml:"runtime"`
	Inputs  []Inputs  `yaml:"inputs"`
	Outputs []Outputs `yaml:"outputs"`
}

type processDescription struct {
	Info    `json:"info"`
	Inputs  []Inputs  `json:"inputs"`
	Outputs []Outputs `json:"outputs"`
	Links   []Link    `json:"links"`
}

type Link struct {
	Href  string `yaml:"href" json:"href"`
	Rel   string `yaml:"rel,omitempty" json:"rel,omitempty"`
	Type  string `yaml:"type,omitempty" json:"type,omitempty"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
}

type Info struct {
	Version            string   `yaml:"version" json:"version"`
	ID                 string   `yaml:"id" json:"id"`
	Title              string   `yaml:"title" json:"title"`
	Description        string   `yaml:"description" json:"description"`
	JobControlOptions  []string `yaml:"jobControlOptions" json:"jobControlOptions"`
	OutputTransmission []string `yaml:"outputTransmission" json:"outputTransmission"`
}

type ValueDefinition struct {
	AnyValue       bool     `yaml:"anyValue"`
	PossibleValues []string `yaml:"possibleValues"`
}

type LiteralDataDomain struct {
	DataType        string          `yaml:"dataType"`
	ValueDefinition ValueDefinition `yaml:"valueDefinition" json:",omitempty"`
}

type Input struct {
	LiteralDataDomain LiteralDataDomain `yaml:"literalDataDomain"`
}

type Inputs struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	Input     Input  `yaml:"input"`
	MinOccurs int    `yaml:"minOccurs"`
	MaxOccurs int    `yaml:"maxOccurs,omitempty"`
}

type Output struct {
	Formats []string `yaml:"transmissionMode"`
}

type Outputs struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Output  Output `yaml:"output"`
	InputID string `yaml:"inputId"` //json omit
}

// Non-OGC types used for this API's implementation of the standard
// Runtime provides information on the host calling the container
type Runtime struct {
	Repository  string   `yaml:"repository"`
	Image       string   `yaml:"image"`
	Tag         string   `yaml:"tag"`
	Provider    Provider `yaml:"provider"`
	Description string   `yaml:"description"`
	EnvVars     []string `yaml:"envVars"`
	Command     []string `yaml:"command"`
}

// Provider is currently limited to AWS Batch, will require changes
// for extending to additional cloud services (e.g. lambda) or controllers (e.g. Azure)
type Provider struct {
	Type          string `yaml:"type"`
	JobDefinition string `yaml:"jobDefinition"`
	JobQueue      string `yaml:"jobQueue"`
	Name          string `yaml:"name"`
}

func (p process) createLinks() []Link {
	links := make([]Link, 0)
	if p.Runtime.Repository != "" {
		links = append(links, Link{Href: fmt.Sprintf("%s/%s", p.Runtime.Repository, p.Runtime.Image)})
	}
	return links
}

func (p process) Describe() (processDescription, error) {
	pd := processDescription{
		Info: p.Info, Inputs: p.Inputs, Outputs: p.Outputs, Links: p.createLinks()}

	return pd, nil
}

func (p process) Type() string {
	return p.Runtime.Provider.Type
}

func (p process) ImgTag() string {
	return fmt.Sprintf("%s:%s", p.Runtime.Image, p.Runtime.Tag)
}

type inpOccurance struct {
	occur    int
	minOccur int
	maxOccur int
}

func (p process) VerifyInputs(inp map[string]interface{}) error {

	requestInp := make(map[string]*inpOccurance)

	for _, i := range p.Inputs {
		requestInp[i.ID] = &inpOccurance{0, i.MinOccurs, i.MaxOccurs}
	}

	for k, val := range inp {
		o, ok := requestInp[k]
		if ok {
			switch v := val.(type) {
			case []interface{}:
				o.occur = len(v)
			default:
				o.occur = 1
			}
		} else {
			return fmt.Errorf("%s is not a valid input option for this process, use /processes/%s endpoint to get list of input options", k, p.Info.ID)
		}
	}

	for id, oc := range requestInp {
		if (oc.maxOccur > 0 && oc.occur > oc.maxOccur) || (oc.occur < oc.minOccur) {
			return errors.New("Not the correct number of occurance of input: " + id)
		}
	}

	return nil
}

// ProcessList describes processes
type ProcessList []process

// ListAll returns all the processes' info
func (ps *ProcessList) ListAll() ([]Info, error) {
	var results []Info
	for _, p := range *ps {
		results = append(results, p.Info)
	}
	return results, nil
}

func (ps *ProcessList) Get(processID string) (process, error) {
	for _, p := range *ps {
		if p.Info.ID == processID {
			return p, nil
		}
	}
	return process{}, errors.New("process not found")
}

func newProcess(f string) (process, error) {
	var p process
	data, err := os.ReadFile(f)
	if err != nil {
		return p, err
	}
	err = yaml.Unmarshal(data, &p)
	if err != nil {
		return p, err
	}
	return p, nil
}

// Load all processes from yml files in the given directory and subdirectories
func LoadProcesses(dir string) (ProcessList, error) {
	processes := make(ProcessList, 0)
	ymls, err := filepath.Glob(fmt.Sprintf("%s/*/*.yml", dir))

	for _, y := range ymls {
		p, err := newProcess(y)
		if err != nil {
			return processes, err
		}
		processes = append(processes, p)
	}

	if err != nil {
		return processes, err
	}
	return processes, err
}
