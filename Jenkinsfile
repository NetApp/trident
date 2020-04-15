@Library('tools@stable/v20.01') _

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

        def max_parallel_stages = 10
        if (env.MAX_PARALLEL_STAGES) {
            max_parallel_stages = env.MAX_PARALLEL_STAGES
        }

        echo "Calling create stages"
        stages = create_stages(
          parallelism: max_parallel_stages,
          repository: 'trident',
        )
      } catch(Exception e) {

        error 'Create-Stages: ' + e.getMessage()

      }
    }

    execute_parallel_stages(
       stages: stages
    )

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
