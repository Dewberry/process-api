import sys
import json


def parse_input(params_string: str) -> str:
    try:
        params = json.loads(params_string)
        if not params["text"]:
            raise ValueError("input json must include a `text` field")
        return params["text"]
    except Exception as e:
        print("Error:", str(e))
        sys.exit(1)


def print_plugin_results(msg: str):
    print({"plugin_results": {"message": msg}})


if __name__ == "__main__":
    """
    exptected arg: '{"jobID": "sadf234sdf234sdf", "text": "hello!"}'
    expected response: {"plugin_outputs": {"message": "hello! from pyecho"}}
    """
    print("initializing pyecho plugin")
    if len(sys.argv) == 2 and (sys.argv[1] == "--help" or sys.argv[1] == "-h"):
        print(__doc__)
        sys.exit()

    if len(sys.argv) != 2:
        print(
            """Error: required input missing. \example usage: main.py '{"jobID": "sadf234sdf234sdf", "text": "hello!"}'"""
        )
        sys.exit(1)

    params_string = sys.argv[-1]

    try:
        message = parse_input(params_string)
    except Exception as e:
        print(e)
        sys.exit(1)

    try:
        print_plugin_results(message)
    except Exception as e:
        print(e)
        sys.exit(1)
