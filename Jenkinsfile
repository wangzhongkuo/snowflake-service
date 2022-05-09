#!/usr/bin/env groovy

dockerImage = null

reportOutput = 'reports'

pipeline {
    agent {
        label 'os:linux'
    }
    options {
        skipDefaultCheckout()
        disableConcurrentBuilds()
        buildDiscarder(logRotator(
            daysToKeepStr: '15'
        ))
        ansiColor('xterm')
    }
    parameters {
        booleanParam(name: 'CLEAN_WS',
            defaultValue: false,
            description: 'When checked, will clean workspace.')
        booleanParam(name: 'AUTO_DEPLOY',
            defaultValue: false,
            description: 'When checked, will automatically deploy to dev environment.')
    }
    stages {
        stage('Clean') {
            steps {
                script {
                    if (params.CLEAN_WS) {
                        cleanWs()
                    }
                    sh "rm -rf ${reportOutput}"
                }
            }
        }
        stage('Checkout') {
            steps {
                checkout scm
            }
        }
        stage('Unit Test') {
            steps {
                sh "docker buildx build --build-arg REPORT_OUTPUT=${reportOutput} --target reports --output ${reportOutput} ."
                archiveArtifacts artifacts: "${reportOutput}/*.*"
                junit "${reportOutput}/test.xml"
            }
        }
        stage('Sonar Scan') {
            steps {
                script {
                    lintReportFile = goLintRun()

                    def projectKey = "infra-snowflake-service"
                    def optionalProperties = [
                        "sonar.sources=.",
                        "sonar.exclusions=**/*.pb.go",
                        "sonar.tests=.",
                        "sonar.test.inclusions=**/*_test.go",
                        "sonar.go.golangci-lint.reportPaths=${lintReportFile}",
                        "sonar.go.tests.reportPaths=${reportOutput}/test.json",
                        "sonar.go.coverage.reportPaths=${reportOutput}/coverage.out",
                    ]

                    sonarScan projectKey: projectKey, optionalProperties: optionalProperties, abortPipeline: true
                }
            }
        }
        stage('Docker Build') {
            steps {
                script {
                    dockerImage = dockerBuild project: 'infra', repo: 'snowflake-service', push: true, multiArch: true
                    echo "Built docker image: ${dockerImage}"
                }
            }
        }
        stage('Deploy') {
            when {
                expression { return params.AUTO_DEPLOY }
            }
            steps {
                autoDeploy targetJob: 'SDK/Ops/Deploy/snowflake-service', image: dockerImage, docker: true
            }
        }
    }
}
