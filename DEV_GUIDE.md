## Config
- All secrets and configuration settings are handled through environment variables
- There is an example.env provided to ease the configuration process
- Command line flags are available for config that is only needed at startup, they take precedence over the environment variables when used. Here is the resolution order:

    - Flag, where option is available and used
    - Environment variable
    - Default value, where available
