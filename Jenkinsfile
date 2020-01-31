@Library('tools@stable/v19.10') _

node {

  def error_message = ''
  def slack_result = ''
  def ssh_options=(
    '-o StrictHostKeyChecking=no ' +
    '-o UserKnownHostsFile=/dev/null ' +
    '-o ServerAliveInterval=60 -i tools/' +
    env.SSH_PRIVATE_KEY_PATH +
    ' -q'
  )

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
        stages = create_stages(
          parallelism: env.MAX_PARALLEL_STAGES,
          repository: 'trident',
          ssh_options: ssh_options
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
