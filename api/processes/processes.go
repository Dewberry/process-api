// Package processes register processes from yaml specs
// and provide types and function to interact with these processes
package processes

import (
	"app/controllers"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/labstack/gommon/log"
	"gopkg.in/yaml.v3"
)

type Process struct {
	Info      Info      `yaml:"info" json:"info"`
	Host      Host      `yaml:"host" json:"host"`
	Container Container `yaml:"container" json:"container"`
	Inputs    []Inputs  `yaml:"inputs" json:"inputs"`
	Outputs   []Outputs `yaml:"outputs" json:"outputs"`
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
	AnyValue       bool     `yaml:"anyValue" json:"anyValue"`
	PossibleValues []string `yaml:"possibleValues" json:"possibleValues"`
}

type LiteralDataDomain struct {
	DataType        string          `yaml:"dataType" json:"dataType"`
	ValueDefinition ValueDefinition `yaml:"valueDefinition" json:"valueDefinition,omitempty"`
}

type Input struct {
	LiteralDataDomain LiteralDataDomain `yaml:"literalDataDomain" json:"literalDataDomain"`
}

type Inputs struct {
	ID          string `yaml:"id" json:"id"`
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	Input       Input  `yaml:"input" json:"input"`
	MinOccurs   int    `yaml:"minOccurs" json:"minOccurs"`
	MaxOccurs   int    `yaml:"maxOccurs,omitempty" json:"maxOccurs,omitempty"`
}

type Output struct {
	Formats []string `yaml:"transmissionMode" json:"transmissionMode"`
}

type Outputs struct {
	ID          string `yaml:"id" json:"id"`
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	Output      Output `yaml:"output" json:"output"`
	InputID     string `yaml:"inputId" json:"inputId,omitempty"`
}

type Resources struct {
	CPUs   float32 `yaml:"cpus" json:"cpus,omitempty"`
	Memory int     `yaml:"memory" json:"memory,omitempty"`
}

type Host struct {
	Type          string `yaml:"type" json:"type"`
	JobDefinition string `yaml:"jobDefinition" json:"jobDefinition,omitempty"`
	JobQueue      string `yaml:"jobQueue" json:"jobQueue,omitempty"`
}

type Container struct {
	Image     string    `yaml:"image" json:"image"`
	EnvVars   []string  `yaml:"envVars" json:"envVars,omitempty"`
	Command   []string  `yaml:"command" json:"command,omitempty"`
	Resources Resources `yaml:"maxResources" json:"maxResources,omitempty"`
}

func (p Process) Type() string {
	return p.Host.Type
}

type inpOccurance struct {
	occur    int
	minOccur int
	maxOccur int
}

func (p Process) VerifyInputs(inp map[string]interface{}) error {

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

func (p Process) VerifyLocalEnvars(container Container) error {
	var missingEnvVars []string
	for _, envVar := range container.EnvVars {
		if os.Getenv(envVar) == "" {
			missingEnvVars = append(missingEnvVars, envVar)
		}
	}
	if len(missingEnvVars) > 0 {
		return fmt.Errorf("error: env variables not found: %v. please restart the server with these in place", missingEnvVars)
	}
	return nil
}

// ProcessList describes processes
// This is not a map since ProcessList Handler function wants order
type ProcessList struct {
	List     []Process
	InfoList []Info
}

func (ps *ProcessList) Get(processID string) (Process, int, error) {
	for i, p := range (*ps).List {
		if p.Info.ID == processID {
			return p, i, nil
		}
	}
	return Process{}, 0, errors.New("process not found")
}

func MarshallProcess(f string) (Process, error) {
	var p Process
	data, err := os.ReadFile(f)
	if err != nil {
		return p, err
	}
	err = yaml.Unmarshal(data, &p)
	if err != nil {
		return Process{}, err
	}

	// if processes is AWS Batch process get its resources, image, etc
	// the problem with doing this here is that if the job definition is updated while we are doing this, our process info will not update
	switch p.Host.Type {
	case "aws-batch":
		c, err := controllers.NewAWSBatchController(os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY"), os.Getenv("AWS_REGION"))
		if err != nil {
			return Process{}, err
		}
		jdi, err := c.GetJobDefInfo(p.Host.JobDefinition)
		if err != nil {
			return Process{}, err
		}
		p.Container.Image = jdi.Image
		p.Container.Resources.Memory = jdi.Memory
		p.Container.Resources.CPUs = jdi.VCPUs
	}

	return p, nil
}

// Load all processes from yml files in the given directory and subdirectories
func LoadProcesses(dir string) (ProcessList, error) {
	var pl ProcessList

	ymls, err := filepath.Glob(fmt.Sprintf("%s/*/*.yml", dir))
	if err != nil {
		return pl, err
	}
	yamls, err := filepath.Glob(fmt.Sprintf("%s/*/*.yaml", dir))
	if err != nil {
		return pl, err
	}
	allYamls := append(ymls, yamls...)
	processes := make([]Process, len(allYamls))

	for i, y := range allYamls {
		p, err := MarshallProcess(y)
		if err != nil {
			log.Errorf("could not register process %s Error: %v", filepath.Base(y), err)
		}
		processes[i] = p
	}

	infos := make([]Info, len(processes))
	for i, p := range processes {
		infos[i] = p.Info
	}

	pl.List = processes
	pl.InfoList = infos

	return pl, err
}
