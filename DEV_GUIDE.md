## Config
- All secrets and configuration settings are handled through environment variables
- There is an example.env provided to ease the configuration process
- Command line flags are available for config that is only needed at startup, they take precedence over the environment variables when used.
- Other configs are defined through env variables so that they can be modified without restarting the server.
- Here is the resolution order:

    - Flag, where option is available and used
    - Environment variable
    - Default value, where available


## Auth
- The middleware validate and parse JWT to inject `X-ProcessAPI-User-Email` and `X-ProcessAPI-User-Roles` headers.
- A user can use tools like Postman to set these headers themselves, but if auth is enabled, they will be overwritten. This setup allows adding submitter info to the database when auth is not enabled.
- API assumes if auth is enabled, every user at least has one role.