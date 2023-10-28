import sys
from papipyplug import parse_input, print_results, plugin_logger

PLUGIN_PARAMS = {"required": ["text"], "optional": []}


def echo(params: dict):
    output_string = params["text"]
    return output_string


if __name__ == "__main__":
    """
    example: python main.py '{"text":"hello"}'
    """
    # Start plugin logger
    plugin_logger()

    # Read, parse, and verify input parameters
    input_params = parse_input(sys.argv, PLUGIN_PARAMS)

    # Add main function here
    results = echo(input_params)

    # Print Results
    print_results(results)