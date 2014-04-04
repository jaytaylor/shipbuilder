Creating an app: Python example
===============================

Step 1: Add your preferred (WSGI)[http://en.wikipedia.org/wiki/Web_Server_Gateway_Interface] server to `requirements.txt`
-------------------------------------------------------------------------------------------------------------------------

e.g. for (Gunicorn)[http://gunicorn.org]:

    echo 'gunicorn==18.0' >> requirements.txt

e.g. for (uWSGI)[http://projects.unbit.it/uwsgi/]:

    echo 'uwsgi==2.0.3' >> requirements.txt


Step 2: Create a `Procfile` for your app and commit it to your repository
-------------------------------------------------------------------------

Example Procfile #1, python app with django and gunicorn:

    web: gunicorn -w $(test -n "${GUNICORN_WORKERS}" && echo $GUNICORN_WORKERS || echo 1) -b 0.0.0.0:$(test -n "${PORT}" && echo $PORT || echo 8000) app.wsgi:application
    scheduler: python app/manage.py celerybeat --loglevel=INFO
    worker: python app/manage.py celeryd -E -Q default --hostanme=$PS.shipbuilder.io --maxtasksperchild=50 --loglevel=INFO


Example Procfile #2, python app with django and uwsgi:

    web: uwsgi --chdir=$(pwd) --module=app.wsgi:application --env --master --pidfile=/tmp/django-master.pid --http=0.0.0.0:${PORT:-'8000'} --processes=${UWSGI_PROCESSES:-'1'} --harakiri=60 --max-requests=500 --vacuum --static-map="/static=$(pwd)/app/static" --http-raw-body --post-buffering 1 --logformat='%(addr) "%(method) %(uri) %(proto)" %(status) %(size) %(msecs)ms "%(referer)" "%(uagent)"'
    scheduler: python app/manage.py celerybeat --loglevel=INFO
    worker: python app/manage.py celeryd -E --hostname=$PS.shipbuilder.io --maxtasksperchild=50 --loglevel=INFO


Example Procfile #3, python app with Flask and uWSGI:

    web: web: uwsgi --chdir=$(pwd) --module=prod:app --master --pidfile=/tmp/flask-master.pid --http=0.0.0.0:${PORT:-'8009'} --processes=${UWSGI_PROCESSES:-'1'} --harakiri=180 --max-requests=500 --vacuum --static-map="/static=$(pwd)/static" --http-raw-body --post-buffering 1 --logformat='%(addr) "%(method) %(uri) %(proto)" %(status) %(size) %(msecs)ms "%(referer)" "%(uagent)"'


For more information it is worth reviewing the (Heroku Procfile documentation)[https://devcenter.heroku.com/articles/procfile].


Step 3: Commit your Procfile to git
-----------------------------------

    git add Procfile
    git commit -m 'Added Procfile for Shipbuilder PaaS compatibility.'


Step 4: Create and configure your shiny new ShipBuilder app
-----------------------------------------------------------

Now you are ready to create and setup your shipbuilder app and deploy the first version.

    # Create the app.
    shipbuilder apps:create MyApp python

    # Set any required environment variables.
    shipbuilder config:set DATABASE_URL='postgres://example.com:5432/mydb' TWILIO_APP_SID='SOME_APP_SID' -aMyApp

    # Add one or more domain names so you can reach your app.
    shipbuilder domains:add myapp.shipbuilder.io -aMyApp


Step 5: Add git remotes for deployment capability
-------------------------------------------------

    # Add a git remote so you can deploy your app.
    cd /path/to/MyApp
    git remote add sb ssh://ubuntu@[YOUR-SB-SERVER-DOMAIN-OR-IP-GOES-HERE]/git/MyApp


Step 6: Deploy your app!
------------------------

    git push -f sb master:master

