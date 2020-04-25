# pbxlog
Query the Samsung OfficeServ PABX to extra the SMDR / call records and write them to an SQL database (sqlite3). Optionally a simple web interface can be run on top of this database allowing viewing of the log.

This has been tested on an OfficeServ 7200. I suspect that it would work with similar models.

Configuration:
* The pabx and calls-db configuration items are mandatory. 
* Everthing else is optional. 
* If webui IP/port is supplied then the webserver will be started.


Screenshot showing database dump:
![PABX database records](screenshot-db.png)


Screenshot showing optional webserver output:
![PABX log records](screenshot-web.png)
