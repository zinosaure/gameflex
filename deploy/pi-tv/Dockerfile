FROM python:3.12-slim
WORKDIR /app
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY tools/pi-tv/server.py tools/pi-tv/server.py
COPY deploy/pi-tv/compatibility.json deploy/pi-tv/compatibility.json
USER 1000:1000
CMD ["python3", "tools/pi-tv/server.py"]
