#!/usr/bin/env python3
"""
Generate full CreatePipeline message structure using pypsrp.
"""

from pypsrp.complex_objects import (
    Pipeline, Command, ApartmentState, HostInfo, RemoteStreamOptions
)
from pypsrp.messages import CreatePipeline
from pypsrp.serializer import Serializer
import xml.etree.ElementTree as ET
import xml.dom.minidom


def pretty_print_xml(xml_string):
    """Pretty print XML for readability."""
    dom = xml.dom.minidom.parseString(xml_string)
    return dom.toprettyxml(indent="  ")


def main():
    # Create serializer
    serializer = Serializer()
    
    # Create pipeline with Get-Date command
    pipeline = Pipeline()
    cmd = Command(cmd="Get-Date")
    pipeline.commands = [cmd]
    
    # Create HostInfo (default values)
    host_info = HostInfo()
    
    # Create the full CreatePipeline message with default values
    # (similar to our Go implementation)
    create_pipeline = CreatePipeline(
        no_input=True,  # Not accepting input
        apartment_state=ApartmentState(value=2),  # Unknown = 2
        remote_stream_options=RemoteStreamOptions(value=15),  # AddInvocationInfo = 15
        add_to_history=False,
        host_info=host_info,
        pipeline=pipeline,
        is_nested=False,
    )
    
    # Serialize to CLIXML
    element = serializer.serialize(create_pipeline)
    
    # Convert to string
    xml_bytes = ET.tostring(element, encoding='unicode', method='xml')
    
    print("=" * 80)
    print("FULL CREATE_PIPELINE MESSAGE (pypsrp Gold Standard):")
    print("=" * 80)
    print(pretty_print_xml(xml_bytes))


if __name__ == "__main__":
    main()
