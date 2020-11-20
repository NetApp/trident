@Library('tools@stable/v20.04') _

node {

  def error_message = ''
  def slack_result = ''

  try {

    stage('Setup') {
      setup(
        repository: 'trident'
      )
    }

    def stages = []
    stage('Create-Stages') {
      try {

        echo "Calling create stages"

        def max_parallel_stages = 10
        if (env.MAX_PARALLEL_STAGES) {
            max_parallel_stages = env.MAX_PARALLEL_STAGES
        }

        stages = create_stages(
          parallelism: max_parallel_stages,
          repository: 'trident'
        )

      } catch(Exception e) {

        error 'Create-Stages: ' + e.getMessage()

      }
    }

    execute_parallel_stages(
       stages: stages
    )

    if (env.STOP_ON_STAGE_FAILURES == 'false') {
      stage('Check-Status') {
        if (process_status_files() == false) {
          failures = readFile file: 'failure_report.txt'
          error 'Check-Status: One or more stages have failed\n' + failures
        }
      }
    }

    stage('Propagate') {
      propagate_changes(
        stage: 'Propagate'
      )
      archive_to_seclab(
        stage: 'Propagate'
      )
    }

    slack_result = 'SUCCESS'

  } catch(Exception e) {

    slack_result = 'FAILURE'
    error_message = e.getMessage()
    error error_message

  } finally {

    stage('Cleanup') {

      cleanup(
        error_message: error_message,
        slack_result: slack_result
      )

    }

  }

}
