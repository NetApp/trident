@Library('tools@master') _

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

    // If we get this far propagate changes to the public trident repo
    stage('Propagate-Changes') {
      if (env.DEPLOY_TRIDENT) {
        echo "Skipping change propagation because DEPLOY_TRIDENT=true"
      } else if (env.BLACK_DUCK_SCAN) {
        echo "Skipping change propagation because BLACK_DUCK_SCAN=true"
      } else {
         propagate_changes()
      }
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
