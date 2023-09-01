package utils

import (
	"encoding/json"
	"fmt"
)

type JobPayload struct {
	JobID  string `json:"jobID"`
	Prefix string `json:"prefix"`
}

func (jp JobPayload) PrintPreviw() {
	fmt.Println("")
	fmt.Println("--------Reading Job Payload----------")
	fmt.Println("")
	fmt.Println("Job ID:", jp.JobID)
	fmt.Println("Prefix:", jp.Prefix)

}

type Configuration struct {
	SimulationName string   `json:"simulationName"`
	Inputs         []string `json:"inputs"`
	Outputs        []string `json:"outputs"`
	JobRootPrefix  string   `json:"jobRootPrefix"`
}

func (config Configuration) PluginResults() ([]byte, error) {
	pluginResults := map[string]interface{}{"plugin_results": config.Outputs}
	return json.Marshal(pluginResults)
}

func (config Configuration) Check() {
	fmt.Println("")
	fmt.Println("---------Configuration Summary----------")
	fmt.Println("SimulationName:", config.SimulationName)
	fmt.Println("Inputs:", config.Inputs)
	fmt.Println("Outputs:", config.Outputs)
	fmt.Println("JobRootPrefix:", config.JobRootPrefix)
}
