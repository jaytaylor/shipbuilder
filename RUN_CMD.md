shipbuilder `run` command

The ShipBuilder console is now easy and straightforward to use:

To use it, run:

$ sb run <shellCommand> -aYOUR-APP

e.g.

$ sb run app/manage.py shell -as1

GOTCHAs to watch out for:
The arrow keys don't work properly, so type your command right the first time (backspace works)
Pressing ctrl-c will quit the shipbuilder app, which will also terminate your container session.

