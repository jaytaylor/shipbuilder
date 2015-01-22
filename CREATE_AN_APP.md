HOWTO: Create a ShipBuilder Application

Pre-requirements

If your app has web workers, it must to have a reachable index page. The http status code of `curl -v http://the-app/` must be in the 2XX or 3XX range. HAProxy uses this url to check the status of the app server, and 4XX or 5XX range status codes will cause the proxy to think the app is down/unavailable and you will not be able to reach it from the web.

Create the app and add git remotes

# Step 1: Create your app
# -------

    sb create [the-app] python

# Step 2: Set your applications config
# -------
# Set your application configuration's key/value pairs.
# Ensure any values with spaces, question marks, quotes, dollars, or
# other characters with special meaning in the shell are surrounded
# by single quotes.

    sb config:set a='b' c=d e=f \
    HTTP_DEPLOYHOOK_URL='http://hipchatz.com?secret=383838' \
    MAINTENANCE_PAGE_URL='http://mycompany.com' -athe-app

# Step 3: Add domain names
# -------

    sb domains:add some-domain.mycompany.com -athe-app

# Step 4: Set the scale for application processes
# -------

    sb ps:scale web=2 worker=2 -athe-app

# Step 5: Add git remote

    git remote add the-app ssh://ubuntu@sb.mycompany.com/git/the-app

