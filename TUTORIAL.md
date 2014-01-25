Creating an app
---------------

First, create a `Procfile` for your app and commit it to your repository.  See: https://devcenter.heroku.com/articles/procfile


Example Procfile #1, python app with django and gunicorn:

    web: gunicorn -w $(test -n "${GUNICORN_WORKERS}" && echo $GUNICORN_WORKERS || echo 1) -b 0.0.0.0:$(test -n "${PORT}" && echo $PORT || echo 8000) app.wsgi:application
    worker: python app/manage.py celeryd -E -Q default --hostanme=$PS.shipbuilder.io --maxtasksperchild=50 --loglevel=INFO


Example Procfile #2, python app with django and uwsgi:

	web: uwsgi --chdir=$(pwd) --module=app.wsgi:application --env --master --pidfile=/tmp/django.pid --http=0.0.0.0:${PORT:-'8000'} --processes=${UWSGI_PROCESSES:-'1'} --harakiri=60 --max-requests=500 --vacuum --static-map="/static=$(pwd)/static" --http-raw-body --uid=1000 --gid=1000
	scheduler: python app/manage.py celerybeat --loglevel=INFO
	worker: python app/manage.py celeryd -E --hostname=$PS.shipbuilder.io --maxtasksperchild=50 --loglevel=INFO


`git add Procfile`
`git commit -m "Added Procfile."`

Then you are ready to create and setup your shipbuilder app and do your first deployment:

    shipbuilder apps:create MyApp python

    shipbuilder config:set DATABASE_URL='postgres://example.com:5432/mydb' TWILIO_APP_SID='SOME_APP_SID' -aMyApp

    shipbuilder domains:add sb-staging.sendhub.com -aMyApp

    cd path/to/MyApp

    git remote add sb ssh://ubuntu@YOUR-SB-SERVER/git/MyApp

    git push -f sb

