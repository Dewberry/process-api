// Code generated by swaggo/swag. DO NOT EDIT.

package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {
            "name": "Seth Lawler",
            "email": "slawler@dewberry.com"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/": {
            "get": {
                "description": "[LandingPage Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_landing_page)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "info"
                ],
                "summary": "Landing Page",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "object",
                            "additionalProperties": true
                        }
                    }
                }
            }
        },
        "/conformance": {
            "get": {
                "description": "[Conformance Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_conformance_classes)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "info"
                ],
                "summary": "API Conformance List",
                "responses": {
                    "200": {
                        "description": "conformsTo:[\"http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/....\"]",
                        "schema": {
                            "type": "object",
                            "additionalProperties": true
                        }
                    }
                }
            }
        },
        "/jobs": {
            "get": {
                "description": "[Job List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "jobs"
                ],
                "summary": "Summary of all (cached) Jobs",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/jobs.JobRecord"
                            }
                        }
                    }
                }
            }
        },
        "/jobs/{jobID}": {
            "get": {
                "description": "[Job Results Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_retrieve_job_results)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "jobs"
                ],
                "summary": "Job Results",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/jobs.jobResponse"
                        }
                    }
                }
            },
            "delete": {
                "description": "[Dismss Job Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#ats_dismiss)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "jobs"
                ],
                "summary": "Dismiss Job",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/jobs.jobResponse"
                        }
                    }
                }
            }
        },
        "/jobs/{jobID}/logs": {
            "get": {
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "jobs"
                ],
                "summary": "Job Logs",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/jobs.JobLogs"
                        }
                    }
                }
            }
        },
        "/processes": {
            "get": {
                "description": "[Process List Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_list)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "processes"
                ],
                "summary": "List Available Processes",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/jobs.Info"
                            }
                        }
                    }
                }
            }
        },
        "/processes/{processID}": {
            "get": {
                "description": "[Process Description Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_process_description)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "processes"
                ],
                "summary": "Describe Process Information",
                "parameters": [
                    {
                        "type": "string",
                        "description": "processID",
                        "name": "processID",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/jobs.ProcessDescription"
                        }
                    }
                }
            }
        },
        "/processes/{processID}/execution": {
            "post": {
                "description": "[Execute Process Specification](https://docs.ogc.org/is/18-062r2/18-062r2.html#sc_create_job)",
                "consumes": [
                    "*/*"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "processes"
                ],
                "summary": "Execute Process",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/jobs.jobResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "jobs.Info": {
            "type": "object",
            "properties": {
                "description": {
                    "type": "string"
                },
                "id": {
                    "type": "string"
                },
                "jobControlOptions": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "outputTransmission": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "title": {
                    "type": "string"
                },
                "version": {
                    "type": "string"
                }
            }
        },
        "jobs.Input": {
            "type": "object",
            "properties": {
                "literalDataDomain": {
                    "$ref": "#/definitions/jobs.LiteralDataDomain"
                }
            }
        },
        "jobs.Inputs": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string"
                },
                "input": {
                    "$ref": "#/definitions/jobs.Input"
                },
                "maxOccurs": {
                    "type": "integer"
                },
                "minOccurs": {
                    "type": "integer"
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "jobs.JobLogs": {
            "type": "object",
            "properties": {
                "api_log": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "container_log": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        },
        "jobs.JobRecord": {
            "type": "object",
            "properties": {
                "commands": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "jobID": {
                    "type": "string"
                },
                "processID": {
                    "type": "string"
                },
                "status": {
                    "type": "string"
                },
                "type": {
                    "type": "string",
                    "default": "process"
                },
                "updated": {
                    "type": "string"
                }
            }
        },
        "jobs.Link": {
            "type": "object",
            "properties": {
                "href": {
                    "type": "string"
                },
                "rel": {
                    "type": "string"
                },
                "title": {
                    "type": "string"
                },
                "type": {
                    "type": "string"
                }
            }
        },
        "jobs.LiteralDataDomain": {
            "type": "object",
            "properties": {
                "dataType": {
                    "type": "string"
                },
                "valueDefinition": {
                    "$ref": "#/definitions/jobs.ValueDefinition"
                }
            }
        },
        "jobs.Output": {
            "type": "object",
            "properties": {
                "formats": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        },
        "jobs.Outputs": {
            "type": "object",
            "properties": {
                "id": {
                    "type": "string"
                },
                "inputID": {
                    "description": "json omit",
                    "type": "string"
                },
                "output": {
                    "$ref": "#/definitions/jobs.Output"
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "jobs.ProcessDescription": {
            "type": "object",
            "properties": {
                "info": {
                    "$ref": "#/definitions/jobs.Info"
                },
                "inputs": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/jobs.Inputs"
                    }
                },
                "links": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/jobs.Link"
                    }
                },
                "outputs": {
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/jobs.Outputs"
                    }
                }
            }
        },
        "jobs.ValueDefinition": {
            "type": "object",
            "properties": {
                "anyValue": {
                    "type": "boolean"
                },
                "possibleValues": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        },
        "jobs.jobResponse": {
            "type": "object",
            "properties": {
                "jobID": {
                    "type": "string"
                },
                "message": {
                    "type": "string"
                },
                "outputs": {},
                "processID": {
                    "type": "string"
                },
                "status": {
                    "type": "string"
                },
                "type": {
                    "type": "string",
                    "default": "process"
                },
                "updated": {
                    "type": "string"
                }
            }
        }
    },
    "externalDocs": {
        "description": "Schemas",
        "url": "http://schemas.opengis.net/ogcapi/processes/part1/1.0/openapi/schemas/"
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "dev-4.19.23",
	Host:             "http://vap.api.dewberryanalytics.com",
	BasePath:         "/",
	Schemes:          []string{"http"},
	Title:            "Process-API Server",
	Description:      "An OGC compliant process server.",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}