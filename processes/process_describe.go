package processes

type processDescription struct {
	Info      `json:"info"`
	Image     string `json:"image"`
	Resources `json:"maxResources,omitempty"`
	Inputs    []Inputs  `json:"inputs"`
	Outputs   []Outputs `json:"outputs"`
	Links     []Link    `json:"links"`
}

func (p process) Describe() (processDescription, error) {
	pd := processDescription{
		Info: p.Info, Image: p.Runtime.Image, Resources: p.Runtime.Resources, Inputs: p.Inputs, Outputs: p.Outputs} // Links: p.createLinks()

	return pd, nil
}
