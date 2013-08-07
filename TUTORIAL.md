Creating an app:

shipbuilder apps:create admin3 python

shipbuilder config:set DATABASE_URL='postgres://localhost:5432/mydb' SH_UTIL_USE_PERSISTENT_DBLINK='1' TWILIO_APP_SID='SOME_APP_SID' -aadmin3

shipbuilder domains:add sb-staging.sendhub.com -aadmin3

cd my-repository

git remote add sb ssh://ubuntu@sb-staging.sendhub.com/git/admin3

git push -f sb

