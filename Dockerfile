FROM centos:7
ADD entrypoint.sh /srv/
ADD coversocks /srv/coversocks
ENTRYPOINT ["/srv/entrypoint.sh"]
