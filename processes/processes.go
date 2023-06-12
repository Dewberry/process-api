// Package processes register processes from yaml specs
// and provide types and function to interact with these processes
package processes

import (
	"app/controllers"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type process struct {
	Info    Info      `yaml:"info"`
	Host    Host      `yaml:"host"`
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

type Output struct {
	Formats []string `yaml:"transmissionMode"`
}

type Outputs struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Output  Output `yaml:"output"`
	InputID string `yaml:"inputId"` //json omit
}

// Resources
type Resources struct {
	CPUs   float32 `yaml:"cpus" json:"cpus,omitempty"`
	Memory int     `yaml:"memory" json:"memory,omitempty"`
}

// Host is currently limited to "local" or "aws-batch", will require changes
// for extending to additional cloud services (e.g. lambda) or controllers (e.g. Azure)
type Host struct {
	// Type should be one of "local", "aws-batch"
	Type string `yaml:"type"`

	// Host specific jargon
	// AWS
	JobDefinition string `yaml:"jobDefinition"`
	JobQueue      string `yaml:"jobQueue"`
}

// Non-OGC types used for this API's implementation of the standard

// Runtime provides information with which to call the container/job
type Runtime struct {
	// Image is the exact string which docker can use to pull an image
	// Image will be overwritten for batch jobs defined by job defitions
	Image string `yaml:"image"`

	EnvVars   []string  `yaml:"envVars"`
	Command   []string  `yaml:"command"`
	Resources Resources `yaml:"maxResources"` // max resources this process can use
}

// func (p process) createLinks() []Link {
// 	var links []Link
// 	if p.Runtime.Image != "" {
// 		links = append(links, Link{Href: fmt.Sprintf("%s/%s", p.Runtime.Repository, p.Runtime.Image)})
// 	}
// 	return links
// }

func (p process) Type() string {
	return p.Host.Type
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
	results := make([]Info, len(*ps))
	for i, p := range *ps {
		results[i] = p.Info
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
		return process{}, err
	}

	// if processes is AWS Batch process get its resources, image, etc
	// the problem with doing this here is that if the job definition is updated while we are doing this, our process info will not update
	switch p.Host.Type {
	case "aws-batch":
		c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_DEFAULT_REGION"))
		if err != nil {
			return process{}, err
		}
		jdi, err := c.GetJobDefInfo(p.Host.JobDefinition)
		if err != nil {
			return process{}, err
		}
		p.Runtime.Image = jdi.Image
		p.Runtime.Resources.Memory = jdi.Memory
		p.Runtime.Resources.CPUs = jdi.VCPUs
	}

	return p, nil
}

// Load all processes from yml files in the given directory and subdirectories
func LoadProcesses(dir string) (ProcessList, error) {
	ymls, err := filepath.Glob(fmt.Sprintf("%s/*/*.yml", dir))
	yamls, err := filepath.Glob(fmt.Sprintf("%s/*/*.yaml", dir))
	y := append(ymls, yamls...)
	processes := make(ProcessList, len(y))

	for i, y := range y {
		p, err := newProcess(y)
		if err != nil {
			return processes, err
		}
		processes[i] = p
	}

	if err != nil {
		return nil, err
	}
	return processes, err
}
