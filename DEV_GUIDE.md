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
- If auth is enabled some or all routes are protected based on env variable `AUTH_LEVEL` settings.
- The middleware validate and parse JWT to verify `X-ProcessAPI-User-Email` header and inject `X-ProcessAPI-User-Roles` header.
- A user can use tools like Postman to set these headers themselves, but if auth is enabled, they will be checked against the token. This setup allows adding submitter info to the database when auth is not enabled.
- I auth is enabled `X-ProcessAPI-User-Email` header is mandatory.
- Requests from Service Role will not be verified for `X-ProcessAPI-User-Email`.
- Requests from Admin Role are allowed to execute all processes, non-admins must have the role with same name as `processID` to execute that process.
- Requests from Admin Role are allowed to retrieve all jobs information, non admins can only retrieve information for jobs that they submitted.
- Only admins can add/update/delete processes