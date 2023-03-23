package jobs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Process struct {
	Info    Info      `yaml:"info"`
	Runtime Runtime   `yaml:"runtime"`
	Inputs  []Inputs  `yaml:"inputs"`
	Outputs []Outputs `yaml:"outputs"`
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

type Formats struct {
	MimeType string `yaml:"mimeType"`
}

type Output struct {
	Formats []Formats `yaml:"formats"`
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
	EntryPoint  string   `yaml:"entrypoint"`
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

func (p Process) CreateLinks() []Link {
	links := make([]Link, 0)
	if p.Runtime.Repository != "" {
		links = append(links, Link{Href: fmt.Sprintf("%s/%s", p.Runtime.Repository, p.Runtime.Image)})
	}
	return links
}

func (p Process) Describe() (map[string]interface{}, error) {
	return map[string]interface{}{
		"info": p.Info, "inputs": p.Inputs, "outputs": p.Outputs, "links": p.CreateLinks()}, nil
}

func (p Process) Type() string {
	return p.Runtime.Provider.Type
}

func (p Process) ImgTag() string {
	return fmt.Sprintf("%s:%s", p.Runtime.Image, p.Runtime.Tag)
}

type inpOccurance struct {
	occur    int
	minOccur int
	maxOccur int
}

func (p Process) convInpsToCommand(inp []map[string]string) ([]string, []interface{}, error) {

	requestInp := make(map[string]*inpOccurance)

	for _, i := range p.Inputs {
		requestInp[i.ID] = &inpOccurance{0, i.MinOccurs, i.MaxOccurs}
	}

	inpMap := make(map[string][]string)
	for _, i := range inp {
		o := requestInp[i["id"]]
		o.occur++
		if o.occur == 1 {
			inpMap[i["id"]] = []string{i["value"]}
		} else {
			inpMap[i["id"]] = append(inpMap[i["id"]], i["value"])
		}
	}

	for id, oc := range requestInp {
		if (oc.maxOccur > 0 && oc.occur > oc.maxOccur) || (oc.occur < oc.minOccur) {
			return nil, nil, errors.New("Not the correct number of occurance of input: " + id)
		}
	}

	cmd := append([]string{}, p.Runtime.Command...)
	for id, val := range inpMap {
		for i := 0; i < len(cmd); i++ {
			if cmd[i] == "_"+id+"_" {
				cmd = append(cmd[:i], append(val, cmd[i+1:]...)...)
				i += len(val)
			}
		}
	}

	// parse outputs
	var outputs []interface{}
	for _, op := range p.Outputs {
		val, ok := inpMap[op.InputID]
		if ok {
			outputs = append(outputs, Link{Href: val[0], Title: op.ID})
		}
	}

	return cmd, outputs, nil
}

type ProcessList []Process

func (ps *ProcessList) ListAll() ([]Info, error) {
	var results []Info
	for _, p := range *ps {
		p.Info.Description += fmt.Sprintf(" called from docker image (%s:%s)", p.Runtime.Image, p.Runtime.Tag)
		results = append(results, p.Info)
	}
	return results, nil
}

func (ps *ProcessList) Get(processID string) (Process, error) {
	for _, p := range *ps {
		p.Info.Description += fmt.Sprintf(" called from docker image (%s:%s)", p.Runtime.Image, p.Runtime.Tag)
		if p.Info.ID == processID {
			return p, nil
		}
	}
	return Process{}, errors.New("process not found")
}

func newProcess(f string) (Process, error) {
	var p Process
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
