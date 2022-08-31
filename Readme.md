# Agent-P

The easiest way to observe the performance impact of a language agent over time in the tri-state area!

The goal of this app make it as easy as possible to measure the performance over time impact that a language agent has on an application with traffic driven to it. It allows you to easily create batches of jobs that can test a specific use case or performance profile.

## Dependencies

Make sure you have the latest stable version of docker installed on your system.

## Installation

There are [pre-compiled releases](https://github.com/iamemilio/agent-p/releases) for targeted architectures and operating systems available for download. Make sure that you download with safari, since this app is not currently recognized by macos. Then, find the downloaded binary in finder and unzip it. Once unzipped, you must control click it, then press open. Macos will prompt you about whether you trust the application and want to open it, if you say yes you will be able to run the executable. Then, you can put the binary in any vaild bin directory!

## Workflow

A core part of what makes this application easy to work with is the workflow for developing, testing, and implementing a test job for a given use case. The first thing you should do is create a workspace for the content you develop for a specific batch of performance tests.

```sh
mkdir myTestDirectory
```

### Making an Application

This tool should work with any containerized application with an http server listening on it. If your agent is enabled in this application, be sure to configure it to accept environment variables for setup. At run time, the following environment variables will always be injected into your container environment:

- "NEW_RELIC_LICENSE_KEY": a new relic license key
- "NEW_RELIC_APP_NAME": the name of your application in NR1
- "NEW_RELIC_HOST": the connection endpoint your app will send data to

Additional custom environment variables can be injected into the application container in the `environment-variables` list for an application in the config file. We will cover that later.

Once you verify that your application can run in a container, push it to any publicly accessible container image registry. Make sure that the image is public and can be pulled without credentials, and be sure not to put any sensitive data inside of it. Doing things this way allows you to run these tests on any system of your choice, as long as it has the client and a config file. 

### Making a Config

Once you have an application ready, lets make a config. You can generate a boilerplate config file by calling `agent-p create config` and it will create a file named `config.yaml` in your working directory that looks like this.

```sh
agent-p create config
```

```yaml
version: 0.1.0
new-relic-server: production
debug: false
jobs:
  - name: example
    app:
        image: YOUR APP CONTAINER IMAGE
        service-port: 8000
        environment-variables:
            EXAMPLE_KEY: EXAMPLE_VALUE
    traffic-driver:
        service-endpoint: /your_endpoint
        image: quay.io/emiliogarcia_1/traffic-driver:latest
        startup-delay: 20  # time in seconds the traffic driver waits to send traffic to the application
        traffic:
            duration: 120   # time in seconds the traffic driver runs
            requests-per-second: 100  # number of requests sent to the server per second
            concurrent-requests: 3  # number of concurrent requests sent each time a request is sent
```

You **must** replace the following values with your own:
- `app.image`: the location of your image in a public container registry; example: `"quay.io/emiliogarcia_1/example-app:latest"`
- `traffic-driver.service-endpoint`: the http endpoint the traffic driver will hit; example: "/"

**NOTE** that the total number of requests per second is traffic.requests-per-second * traffic.concurrent-requests.

The traffic-driver config allows you to tune the traffic sent to the server. It accepts the following fields:

| field | type | definition |
| --- | --- | --- |
| startup-delay | uint | time in seconds that the traffic driver will wait to send traffic to the app |
| service-endpoint | string | the http endpoint that the traffic driver will send traffic to:  localhost:8000/\<service-endpoint\> |
| traffic.duration | uint | time in seconds that the driver will send traffic to the service endpoint |
| traffic.requests-per-second | uint | the number of requests the driver will make to the service endpoint per second |
| traffic.concurrent-requests | uint | the number of concurrent requests that are allowed to be sent to the server |

### Running

Once your `config.yaml` is ready, all you need to do is run the command:

```sh
agent-p run config.yaml
```

It will create a directory named jobs in your working directory, then for each job in your config file, it will create a directoy
that contains a `docker-compose.yaml` file that defines how that job is ran and data that was gathered during that run in a file named
`data.csv`.

## Troubleshooting

The following tools can help you troubleshoot:

- the `--debug` flag will print a verbose output
- calling `agent-p create jobs` creates jobs without running them, allowing you to verify any issues with the docker-compose at your own pace
- running `agent-p run --no-clean` will leave stopped docker containers on your system, giving you access to their logs
